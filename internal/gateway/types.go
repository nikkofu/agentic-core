package gateway

import (
	"context"
	"encoding/json"
)

type MessageType string

const (
	MessageTypeUnknown  MessageType = "unknown"
	MessageTypeText     MessageType = "text"
	MessageTypeMarkdown MessageType = "markdown"
	MessageTypeImage    MessageType = "image"
	MessageTypeAudio    MessageType = "audio"
	MessageTypeVideo    MessageType = "video"
	MessageTypeFile     MessageType = "file"
	MessageTypeNews     MessageType = "news"
	MessageTypeEvent    MessageType = "event"
	MessageTypeLink     MessageType = "link"
	MessageTypeLocation MessageType = "location"
)

type MessageFormat string

const (
	FormatPlainText MessageFormat = "plain_text"
	FormatMarkdown  MessageFormat = "markdown"
)

type MediaKind string

const (
	MediaKindImage MediaKind = "image"
	MediaKindAudio MediaKind = "audio"
	MediaKindVideo MediaKind = "video"
	MediaKindFile  MediaKind = "file"
)

type MediaItem struct {
	Kind             MediaKind              `json:"kind"`
	MediaID          string                 `json:"media_id,omitempty"`
	FileName         string                 `json:"file_name,omitempty"`
	Path             string                 `json:"path,omitempty"`
	DataBase64       string                 `json:"data_base64,omitempty"`
	URL              string                 `json:"url,omitempty"`
	MIMEType         string                 `json:"mime_type,omitempty"`
	Title            string                 `json:"title,omitempty"`
	Description      string                 `json:"description,omitempty"`
	ThumbnailMediaID string                 `json:"thumbnail_media_id,omitempty"`
	Size             int64                  `json:"size,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

type Article struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	PicURL      string `json:"pic_url,omitempty"`
}

type StandardMessage struct {
	SessionID       string                 `json:"session_id"`
	ChannelName     string                 `json:"channel_name"`
	MessageID       string                 `json:"message_id,omitempty"`
	ParentMessageID string                 `json:"parent_message_id,omitempty"`
	SenderID        string                 `json:"sender_id,omitempty"`
	SenderName      string                 `json:"sender_name,omitempty"`
	ReceiverID      string                 `json:"receiver_id,omitempty"`
	Text            string                 `json:"text,omitempty"`
	MessageType     MessageType            `json:"message_type,omitempty"`
	Format          MessageFormat          `json:"format,omitempty"`
	Media           []MediaItem            `json:"media,omitempty"`
	Articles        []Article              `json:"articles,omitempty"`
	Card            map[string]interface{} `json:"card,omitempty"`
	RawContent      json.RawMessage        `json:"raw_content,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type ChannelRequest = StandardMessage
type ChannelResponse = StandardMessage

// RichAdapter 扩展了旧的纯文本发送接口，允许适配器直接回推统一消息结构。
type RichAdapter interface {
	Adapter
	Send(ctx context.Context, msg ChannelResponse) error
}
