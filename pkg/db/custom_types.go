package db

type TenantSettings struct {
	WelcomeMessage           string `json:"welcomeMessage,omitempty"`
	BotName                  string `json:"botName,omitempty"`
	CancellationMsg          string `json:"cancellationMessage,omitempty"`
	ContactEmail             string `json:"contactEmail,omitempty"`
	OwnerPhone               string `json:"ownerPhone,omitempty"`
	LateCancelHours          int    `json:"lateCancelHours"`          // default: 2
	AutoBlockAfterNoShows    int    `json:"autoBlockAfterNoShows"`    // default: 3
	AutoBlockAfterLateCancel int    `json:"autoBlockAfterLateCancel"` // default: 3
	SendWarningBeforeBlock   bool   `json:"sendWarningBeforeBlock"`
}
