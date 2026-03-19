package dingtalk

import (
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	imsdk "github.com/alibabacloud-go/dingtalk/im_1_0"
	oauthsdk "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	tea "github.com/alibabacloud-go/tea/tea"
)

const (
	defaultDingTalkAPIBaseURL  = "https://api.dingtalk.com"
	defaultDingTalkOAPIBaseURL = "https://oapi.dingtalk.com"

	dingTalkConversationSendPath = "/message/send_to_conversation"
	dingTalkWorkNoticeSendPath   = "/topapi/message/corpconversation/asyncsend_v2"
	dingTalkMediaUploadPath      = "/media/upload"
)

type appAccessToken struct {
	Token     string
	ExpiresIn int64
}

type appConversationRequest struct {
	Sender string
	CID    string
	Msg    map[string]any
}

type appInteractiveCardRequest struct {
	OpenConversationID  string
	ConversationType    int32
	CardTemplateID      string
	OutTrackID          string
	CallbackRouteKey    string
	ChatBotID           string
	CardParamMap        map[string]string
	CardMediaIDParamMap map[string]string
	SupportForward      bool
}

type appWorkNoticeRequest struct {
	AgentID    int64
	UserIDList []string
	DeptIDList []int64
	ToAllUser  bool
	Msg        map[string]any
}

type appUploadMediaRequest struct {
	MediaType string
	FileName  string
	Content   []byte
}

type appAPI interface {
	GetAccessToken(ctx context.Context) (appAccessToken, error)
	SendConversation(ctx context.Context, accessToken string, req appConversationRequest) error
	SendInteractiveCard(ctx context.Context, accessToken string, req appInteractiveCardRequest) error
	SendWorkNotice(ctx context.Context, accessToken string, req appWorkNoticeRequest) error
	UploadMedia(ctx context.Context, accessToken string, req appUploadMediaRequest) (string, error)
}

type appHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type AppClient struct {
	cfg AppConfig
	api appAPI
	id  func() string
	now func() time.Time

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type sdkAppAPI struct {
	cfg         AppConfig
	oauth       *oauthsdk.Client
	im          *imsdk.Client
	httpClient  appHTTPDoer
	oapiBaseURL string
}

type dingTalkTarget struct {
	ConversationID     string
	ConversationSender string
	UserIDs            []string
	DeptIDs            []int64
	ToAllUser          bool
}

type dingTalkAPIResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	MediaID string `json:"media_id,omitempty"`
}

func newAppClient(cfg AppConfig, httpClient *http.Client) (*AppClient, error) {
	api, err := newSDKAppAPI(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return &AppClient{
		cfg: cfg,
		api: api,
		id: func() string {
			return fmt.Sprintf("dingtalk-%d", time.Now().UnixNano())
		},
		now: time.Now,
	}, nil
}

func newSDKAppAPI(cfg AppConfig, httpClient *http.Client) (appAPI, error) {
	cfg = cfg.normalize()
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}

	oauthClient, err := oauthsdk.NewClient(newOpenAPIConfig(cfg.APIBaseURL))
	if err != nil {
		return nil, err
	}
	imClient, err := imsdk.NewClient(newOpenAPIConfig(cfg.APIBaseURL))
	if err != nil {
		return nil, err
	}

	return &sdkAppAPI{
		cfg:         cfg,
		oauth:       oauthClient,
		im:          imClient,
		httpClient:  httpClient,
		oapiBaseURL: firstNonEmpty(strings.TrimSpace(cfg.OAPIBaseURL), defaultDingTalkOAPIBaseURL),
	}, nil
}

func newOpenAPIConfig(baseURL string) *openapi.Config {
	config := &openapi.Config{}
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return config
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return config
	}
	if parsed.Host != "" {
		config.Endpoint = tea.String(parsed.Host)
	}
	if parsed.Scheme != "" {
		config.Protocol = tea.String(strings.ToUpper(parsed.Scheme))
	}
	return config
}

func (c *AppClient) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	target := resolveDingTalkTarget(msg)

	cardReq, cardOK, err := c.buildInteractiveCardRequest(msg, target)
	if err != nil {
		return err
	}

	if !cardOK && !target.hasConversation() && !target.hasWorkNotice() {
		return fmt.Errorf("dingtalk app receive target is required")
	}

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	if cardOK {
		return c.api.SendInteractiveCard(ctx, token, cardReq)
	}

	payload, err := c.buildLegacyMessage(ctx, token, msg)
	if err != nil {
		return err
	}

	if target.hasConversation() {
		return c.api.SendConversation(ctx, token, appConversationRequest{
			Sender: target.ConversationSender,
			CID:    target.ConversationID,
			Msg:    payload,
		})
	}

	if c.cfg.AgentID <= 0 {
		return fmt.Errorf("dingtalk app agent id must be positive")
	}

	return c.api.SendWorkNotice(ctx, token, appWorkNoticeRequest{
		AgentID:    c.cfg.AgentID,
		UserIDList: target.UserIDs,
		DeptIDList: target.DeptIDs,
		ToAllUser:  target.ToAllUser,
		Msg:        payload,
	})
}

func (c *AppClient) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && c.now().Before(c.expiresAt) {
		token := c.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	result, err := c.api.GetAccessToken(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Token) == "" {
		return "", fmt.Errorf("dingtalk app access token is empty")
	}

	expiry := c.now().Add(5 * time.Minute)
	if result.ExpiresIn > 0 {
		safeSeconds := result.ExpiresIn - 60
		if safeSeconds <= 0 {
			safeSeconds = result.ExpiresIn
		}
		expiry = c.now().Add(time.Duration(safeSeconds) * time.Second)
	}

	c.mu.Lock()
	c.accessToken = result.Token
	c.expiresAt = expiry
	c.mu.Unlock()
	return result.Token, nil
}

func (c *AppClient) buildInteractiveCardRequest(msg gateway.ChannelResponse, target dingTalkTarget) (appInteractiveCardRequest, bool, error) {
	cardTemplateID := strings.TrimSpace(firstNonEmpty(
		stringValue(msg.Card["cardTemplateId"]),
		stringValue(msg.Card["card_template_id"]),
		c.cfg.CardTemplateID,
	))
	if cardTemplateID == "" {
		return appInteractiveCardRequest{}, false, nil
	}
	if !target.hasConversation() {
		return appInteractiveCardRequest{}, false, fmt.Errorf("dingtalk interactive card requires conversation target")
	}

	cardParamMap, err := toStringMap(msg.Card["cardParamMap"])
	if err != nil {
		return appInteractiveCardRequest{}, false, err
	}
	cardMediaIDParamMap, err := toStringMap(msg.Card["cardMediaIdParamMap"])
	if err != nil {
		return appInteractiveCardRequest{}, false, err
	}

	conversationType := int32(2)
	if value, ok := int32Value(msg.Card["conversationType"]); ok && value > 0 {
		conversationType = value
	} else if value, ok := int32Value(msg.Metadata["conversationType"]); ok && value > 0 {
		conversationType = value
	}

	supportForward := boolValue(msg.Card["supportForward"])
	if !supportForward {
		if options, ok := mapValue(msg.Card["cardOptions"]); ok {
			supportForward = boolValue(options["supportForward"])
		}
	}

	return appInteractiveCardRequest{
		OpenConversationID:  target.ConversationID,
		ConversationType:    conversationType,
		CardTemplateID:      cardTemplateID,
		OutTrackID:          strings.TrimSpace(firstNonEmpty(stringValue(msg.Card["outTrackId"]), c.id())),
		CallbackRouteKey:    strings.TrimSpace(firstNonEmpty(stringValue(msg.Card["callbackRouteKey"]), c.cfg.CardCallbackRouteKey)),
		ChatBotID:           target.ConversationSender,
		CardParamMap:        cardParamMap,
		CardMediaIDParamMap: cardMediaIDParamMap,
		SupportForward:      supportForward,
	}, true, nil
}

func (c *AppClient) buildLegacyMessage(ctx context.Context, accessToken string, msg gateway.ChannelResponse) (map[string]any, error) {
	if payload, ok, err := buildLegacyCardPayload(msg.Card); ok || err != nil {
		return payload, err
	}

	msgType := effectiveDingTalkMessageType(msg)
	switch msgType {
	case "markdown":
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": strings.TrimSpace(stringValue(msg.Metadata["title"])),
				"text":  msg.Text,
			},
		}, nil
	case "text":
		return map[string]any{
			"msgtype": "text",
			"text": map[string]any{
				"content": msg.Text,
			},
		}, nil
	case "image", "voice", "video", "file":
		if len(msg.Media) == 0 {
			return nil, fmt.Errorf("dingtalk %s message requires media", msgType)
		}
		mediaID, err := c.resolveMediaID(ctx, accessToken, msgType, msg.Media[0])
		if err != nil {
			return nil, err
		}
		body := map[string]any{"media_id": mediaID}
		if msgType == "video" {
			if msg.Media[0].ThumbnailMediaID != "" {
				body["thumb_media_id"] = msg.Media[0].ThumbnailMediaID
			}
			if msg.Media[0].Title != "" {
				body["title"] = msg.Media[0].Title
			}
			if msg.Media[0].Description != "" {
				body["message"] = msg.Media[0].Description
			}
		}
		return map[string]any{
			"msgtype": msgType,
			msgType:   body,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported dingtalk app message type: %s", msgType)
	}
}

func (c *AppClient) resolveMediaID(ctx context.Context, accessToken, msgType string, media gateway.MediaItem) (string, error) {
	if strings.TrimSpace(media.MediaID) != "" {
		return media.MediaID, nil
	}
	content, fileName, err := mediaBytes(media)
	if err != nil {
		return "", err
	}
	return c.api.UploadMedia(ctx, accessToken, appUploadMediaRequest{
		MediaType: msgType,
		FileName:  fileName,
		Content:   content,
	})
}

func resolveDingTalkTarget(msg gateway.ChannelResponse) dingTalkTarget {
	target := dingTalkTarget{
		ConversationID:     strings.TrimSpace(firstNonEmpty(stringValue(msg.Metadata["conversationId"]), stringValue(msg.Metadata["cid"]), msg.SessionID)),
		ConversationSender: strings.TrimSpace(firstNonEmpty(stringValue(msg.Metadata["chatbotUserId"]), stringValue(msg.Metadata["chatBotId"]), stringValue(msg.Metadata["sender"]))),
		UserIDs:            stringListValue(msg.Metadata["dingtalk_userid_list"]),
		DeptIDs:            int64ListValue(msg.Metadata["dingtalk_dept_id_list"]),
		ToAllUser:          boolValue(msg.Metadata["dingtalk_to_all_user"]),
	}

	if len(target.UserIDs) == 0 {
		target.UserIDs = stringListValue(msg.Metadata["userid_list"])
	}
	if len(target.UserIDs) == 0 {
		target.UserIDs = stringListValue(msg.Metadata["receiverUserIdList"])
	}
	if len(target.UserIDs) == 0 {
		target.UserIDs = stringListValue(msg.Metadata["senderStaffId"])
	}
	if len(target.UserIDs) == 0 && strings.TrimSpace(msg.ReceiverID) != "" && !isConversationID(msg.ReceiverID) {
		target.UserIDs = []string{strings.TrimSpace(msg.ReceiverID)}
	}
	if len(target.UserIDs) == 0 && strings.TrimSpace(msg.SessionID) != "" && !isConversationID(msg.SessionID) {
		target.UserIDs = []string{strings.TrimSpace(msg.SessionID)}
	}
	target.UserIDs = uniqueStrings(target.UserIDs)
	target.DeptIDs = uniqueInt64s(target.DeptIDs)
	return target
}

func effectiveDingTalkMessageType(msg gateway.ChannelResponse) string {
	switch msg.MessageType {
	case gateway.MessageTypeMarkdown:
		return "markdown"
	case gateway.MessageTypeImage:
		return "image"
	case gateway.MessageTypeAudio:
		return "voice"
	case gateway.MessageTypeVideo:
		return "video"
	case gateway.MessageTypeFile:
		return "file"
	case gateway.MessageTypeText:
		if msg.Format == gateway.FormatMarkdown {
			return "markdown"
		}
		return "text"
	}
	if msg.Format == gateway.FormatMarkdown {
		return "markdown"
	}
	if len(msg.Media) > 0 {
		switch msg.Media[0].Kind {
		case gateway.MediaKindImage:
			return "image"
		case gateway.MediaKindAudio:
			return "voice"
		case gateway.MediaKindVideo:
			return "video"
		case gateway.MediaKindFile:
			return "file"
		}
	}
	return "text"
}

func buildLegacyCardPayload(card map[string]any) (map[string]any, bool, error) {
	if len(card) == 0 {
		return nil, false, nil
	}

	if payload, ok := mapValue(card["action_card"]); ok {
		return map[string]any{"msgtype": "action_card", "action_card": payload}, true, nil
	}
	if payload, ok := mapValue(card["actionCard"]); ok {
		return map[string]any{"msgtype": "action_card", "action_card": payload}, true, nil
	}
	if payload, ok := mapValue(card["link"]); ok {
		return map[string]any{"msgtype": "link", "link": payload}, true, nil
	}
	if payload, ok := mapValue(card["oa"]); ok {
		return map[string]any{"msgtype": "oa", "oa": payload}, true, nil
	}
	if rawType := normalizeLegacyMsgType(stringValue(card["msgtype"])); rawType != "" {
		payload := cloneAnyMap(card)
		payload["msgtype"] = rawType
		if rawType == "action_card" {
			if value, ok := payload["actionCard"]; ok {
				payload["action_card"] = value
				delete(payload, "actionCard")
			}
		}
		return payload, true, nil
	}
	return nil, false, nil
}

func normalizeLegacyMsgType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "actioncard", "action_card":
		return "action_card"
	case "text", "markdown", "image", "voice", "video", "file", "link", "oa":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func mediaBytes(media gateway.MediaItem) ([]byte, string, error) {
	if strings.TrimSpace(media.Path) != "" {
		content, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, "", err
		}
		name := strings.TrimSpace(media.FileName)
		if name == "" {
			name = filepath.Base(media.Path)
		}
		return content, name, nil
	}
	if strings.TrimSpace(media.DataBase64) != "" {
		content, err := base64.StdEncoding.DecodeString(media.DataBase64)
		if err != nil {
			return nil, "", err
		}
		name := strings.TrimSpace(media.FileName)
		if name == "" {
			name = "upload.bin"
		}
		return content, name, nil
	}
	return nil, "", fmt.Errorf("dingtalk media requires media_id, path or data_base64")
}

func mapValue(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed), true
	default:
		return nil, false
	}
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func toStringMap(value any) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned, nil
	case map[string]any:
		converted := make(map[string]string, len(typed))
		for key, item := range typed {
			converted[key] = stringValue(item)
		}
		return converted, nil
	default:
		return nil, fmt.Errorf("expected map value, got %T", value)
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func int32Value(value any) (int32, bool) {
	switch typed := value.(type) {
	case int:
		return int32(typed), true
	case int32:
		return typed, true
	case int64:
		return int32(typed), true
	case float64:
		return int32(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 32)
		if err != nil {
			return 0, false
		}
		return int32(parsed), true
	default:
		return 0, false
	}
}

func stringListValue(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := stringValue(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		if trimmed := stringValue(value); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	}
}

func int64ListValue(value any) []int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int64:
		return []int64{typed}
	case int:
		return []int64{int64(typed)}
	case float64:
		return []int64{int64(typed)}
	case string:
		parts := strings.Split(typed, ",")
		out := make([]int64, 0, len(parts))
		for _, part := range parts {
			parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil {
				out = append(out, parsed)
			}
		}
		return out
	case []int64:
		return append([]int64(nil), typed...)
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch converted := item.(type) {
			case int:
				out = append(out, int64(converted))
			case int64:
				out = append(out, converted)
			case float64:
				out = append(out, int64(converted))
			case string:
				parsed, err := strconv.ParseInt(strings.TrimSpace(converted), 10, 64)
				if err == nil {
					out = append(out, parsed)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isConversationID(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "cid")
}

func (t dingTalkTarget) hasConversation() bool {
	return strings.TrimSpace(t.ConversationID) != "" && strings.TrimSpace(t.ConversationSender) != ""
}

func (t dingTalkTarget) hasWorkNotice() bool {
	return t.ToAllUser || len(t.UserIDs) > 0 || len(t.DeptIDs) > 0
}

func (s *sdkAppAPI) GetAccessToken(ctx context.Context) (appAccessToken, error) {
	resp, err := s.oauth.GetAccessToken((&oauthsdk.GetAccessTokenRequest{}).
		SetAppKey(s.cfg.ClientID).
		SetAppSecret(s.cfg.ClientSecret))
	if err != nil {
		return appAccessToken{}, err
	}
	if resp == nil || resp.Body == nil || tea.StringValue(resp.Body.AccessToken) == "" {
		return appAccessToken{}, fmt.Errorf("dingtalk app get access token returned empty token")
	}
	return appAccessToken{
		Token:     tea.StringValue(resp.Body.AccessToken),
		ExpiresIn: tea.Int64Value(resp.Body.ExpireIn),
	}, nil
}

func (s *sdkAppAPI) SendInteractiveCard(ctx context.Context, accessToken string, req appInteractiveCardRequest) error {
	request := (&imsdk.SendInteractiveCardRequest{}).
		SetCardTemplateId(req.CardTemplateID).
		SetConversationType(req.ConversationType).
		SetOpenConversationId(req.OpenConversationID).
		SetOutTrackId(req.OutTrackID)

	if req.CallbackRouteKey != "" {
		request.SetCallbackRouteKey(req.CallbackRouteKey)
	}
	if req.ChatBotID != "" {
		request.SetChatBotId(req.ChatBotID)
	}
	request.SetCardData((&imsdk.SendInteractiveCardRequestCardData{}).
		SetCardParamMap(stringPtrMap(req.CardParamMap)).
		SetCardMediaIdParamMap(stringPtrMap(req.CardMediaIDParamMap)))
	if req.SupportForward {
		request.SetCardOptions((&imsdk.SendInteractiveCardRequestCardOptions{}).SetSupportForward(true))
	}

	headers := (&imsdk.SendInteractiveCardHeaders{}).SetXAcsDingtalkAccessToken(accessToken)
	resp, err := s.im.SendInteractiveCardWithOptions(request, headers, &util.RuntimeOptions{})
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil || !tea.BoolValue(resp.Body.Success) {
		return fmt.Errorf("dingtalk interactive card send failed")
	}
	return nil
}

func (s *sdkAppAPI) SendConversation(ctx context.Context, accessToken string, req appConversationRequest) error {
	body, err := json.Marshal(map[string]any{
		"sender": req.Sender,
		"cid":    req.CID,
		"msg":    req.Msg,
	})
	if err != nil {
		return err
	}

	httpReq, err := s.newJSONRequest(ctx, dingTalkConversationSendPath, accessToken, nil, body)
	if err != nil {
		return err
	}
	return s.doAPIRequest(httpReq, "dingtalk conversation send")
}

func (s *sdkAppAPI) SendWorkNotice(ctx context.Context, accessToken string, req appWorkNoticeRequest) error {
	body, err := json.Marshal(map[string]any{
		"agent_id":     req.AgentID,
		"userid_list":  strings.Join(req.UserIDList, ","),
		"dept_id_list": joinInt64s(req.DeptIDList),
		"to_all_user":  req.ToAllUser,
		"msg":          req.Msg,
	})
	if err != nil {
		return err
	}

	httpReq, err := s.newJSONRequest(ctx, dingTalkWorkNoticeSendPath, accessToken, nil, body)
	if err != nil {
		return err
	}
	return s.doAPIRequest(httpReq, "dingtalk work notice send")
}

func (s *sdkAppAPI) UploadMedia(ctx context.Context, accessToken string, req appUploadMediaRequest) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("media", req.FileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(req.Content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	query := url.Values{}
	query.Set("type", req.MediaType)
	httpReq, err := s.newRequest(ctx, http.MethodPost, dingTalkMediaUploadPath, accessToken, query, &buf)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed dingTalkAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk media upload failed: %s (%d)", parsed.ErrMsg, parsed.ErrCode)
	}
	if strings.TrimSpace(parsed.MediaID) == "" {
		return "", fmt.Errorf("dingtalk media upload returned empty media id")
	}
	return parsed.MediaID, nil
}

func (s *sdkAppAPI) doAPIRequest(req *http.Request, action string) error {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var parsed dingTalkAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}
	if parsed.ErrCode != 0 {
		return fmt.Errorf("%s failed: %s (%d)", action, parsed.ErrMsg, parsed.ErrCode)
	}
	return nil
}

func (s *sdkAppAPI) newJSONRequest(ctx context.Context, path, accessToken string, query url.Values, body []byte) (*http.Request, error) {
	req, err := s.newRequest(ctx, http.MethodPost, path, accessToken, query, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (s *sdkAppAPI) newRequest(ctx context.Context, method, path, accessToken string, query url.Values, body io.Reader) (*http.Request, error) {
	if body == nil {
		body = bytes.NewReader(nil)
	}
	base, err := url.Parse(strings.TrimRight(s.oapiBaseURL, "/"))
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	target := base.ResolveReference(ref)
	values := target.Query()
	values.Set("access_token", accessToken)
	for key, items := range query {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	target.RawQuery = values.Encode()
	return http.NewRequestWithContext(ctx, method, target.String(), body)
}

func joinInt64s(values []int64) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func stringPtrMap(values map[string]string) map[string]*string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]*string, len(values))
	for key, value := range values {
		copied := value
		out[key] = &copied
	}
	return out
}
