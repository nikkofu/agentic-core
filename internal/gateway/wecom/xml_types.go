package wecom

import "encoding/xml"

type callbackEnvelope struct {
	XMLName xml.Name `xml:"xml"`
	Encrypt string   `xml:"Encrypt"`
	AgentID int64    `xml:"AgentID"`
}

type callbackMessage struct {
	XMLName        xml.Name `xml:"xml"`
	ToUserName     string   `xml:"ToUserName"`
	FromUserName   string   `xml:"FromUserName"`
	CreateTime     int64    `xml:"CreateTime"`
	MsgType        string   `xml:"MsgType"`
	Content        string   `xml:"Content"`
	MsgID          string   `xml:"MsgId"`
	AgentID        int64    `xml:"AgentID"`
	Event          string   `xml:"Event"`
	ChangeType     string   `xml:"ChangeType"`
	PicURL         string   `xml:"PicUrl"`
	MediaID        string   `xml:"MediaId"`
	ThumbMediaID   string   `xml:"ThumbMediaId"`
	Format         string   `xml:"Format"`
	Title          string   `xml:"Title"`
	Description    string   `xml:"Description"`
	URL            string   `xml:"Url"`
	LocationX      string   `xml:"Location_X"`
	LocationY      string   `xml:"Location_Y"`
	Scale          string   `xml:"Scale"`
	Label          string   `xml:"Label"`
	Recognition    string   `xml:"Recognition"`
	ThumbURL       string   `xml:"ThumbUrl"`
	SelectedItems  string   `xml:"SelectedItems"`
	SelectedTicket string   `xml:"SelectedTicket"`
}
