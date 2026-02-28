package models

type TextObject struct {
	Body string `json:"body"`
}

type ButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type InteractiveReply struct {
	Type        string      `json:"type"`
	ButtonReply ButtonReply `json:"button_reply"`
}

type WhatsAppMessage struct {
	From        string           `json:"from"`
	ID          string           `json:"id"`
	Timestamp   string           `json:"timestamp"`
	Type        string           `json:"type"`
	Text        TextObject       `json:"text"`
	Interactive InteractiveReply `json:"interactive"`
}

type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string            `json:"messaging_product"`
				Metadata         map[string]any    `json:"metadata"`
				Contacts         []any             `json:"contacts"`
				Messages         []WhatsAppMessage `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}
