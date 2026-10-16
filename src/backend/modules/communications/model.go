// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

// CommunicationChannel represents a notification channel configuration.
// Secret fields (webhook_url, token, password) are never included in JSON responses.
type CommunicationChannel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ChannelType    string `json:"channelType"`
	Method         string `json:"method"`
	IsDefault      bool   `json:"isDefault"`
	Enabled        bool   `json:"enabled"`
	ServerURL      string `json:"serverUrl,omitempty"`
	Topic          string `json:"topic,omitempty"`
	ChatID         string `json:"chatId,omitempty"`
	PhoneNumberID  string `json:"phoneNumberId,omitempty"`
	RecipientPhone string `json:"recipientPhone,omitempty"`
	SenderNumber   string `json:"senderNumber,omitempty"`
	Recipients     string `json:"recipients,omitempty"`
	Username       string `json:"username,omitempty"`
	Priority       string `json:"priority,omitempty"`
	// Config carries plugin-owned fields stored in the row's JSON
	// column. For built-in channel types it's empty; for plugin-owned
	// channel types the plugin's ConfigForm writes its own
	// per-channel fields here and the
	// plugin's Test/Send reads them via cfg["bot_token"] etc.
	Config    map[string]string `json:"config,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// CreateChannelInput is the request body for creating a communication channel.
type CreateChannelInput struct {
	Name           string            `json:"name"`
	ChannelType    string            `json:"channelType"`
	Method         string            `json:"method"`
	IsDefault      bool              `json:"isDefault"`
	WebhookURL     string            `json:"webhookUrl"`
	ServerURL      string            `json:"serverUrl"`
	Token          string            `json:"token"`
	Topic          string            `json:"topic"`
	ChatID         string            `json:"chatId"`
	PhoneNumberID  string            `json:"phoneNumberId"`
	RecipientPhone string            `json:"recipientPhone"`
	SenderNumber   string            `json:"senderNumber"`
	Recipients     string            `json:"recipients"`
	Username       string            `json:"username"`
	Password       string            `json:"password"`
	Priority       string            `json:"priority"`
	Config         map[string]string `json:"config"`
}

// UpdateChannelInput is the request body for updating a communication channel.
// Pointer fields allow partial updates.
type UpdateChannelInput struct {
	Name           *string           `json:"name"`
	Method         *string           `json:"method"`
	IsDefault      *bool             `json:"isDefault"`
	Enabled        *bool             `json:"enabled"`
	WebhookURL     *string           `json:"webhookUrl"`
	ServerURL      *string           `json:"serverUrl"`
	Token          *string           `json:"token"`
	Topic          *string           `json:"topic"`
	ChatID         *string           `json:"chatId"`
	PhoneNumberID  *string           `json:"phoneNumberId"`
	RecipientPhone *string           `json:"recipientPhone"`
	SenderNumber   *string           `json:"senderNumber"`
	Recipients     *string           `json:"recipients"`
	Username       *string           `json:"username"`
	Password       *string           `json:"password"`
	Priority       *string           `json:"priority"`
	Config         map[string]string `json:"config"`
}
