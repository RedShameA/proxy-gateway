package profiles

type CreatedCredentialInput struct {
	ID                string
	ProfileID         string
	Remark            string
	Password          string
	ProfileIdentifier string
	Endpoint          string
}

type CreatedCredential struct {
	ID              string `json:"id"`
	AccessProfileID string `json:"access_profile_id"`
	Remark          string `json:"remark"`
	Password        string `json:"password"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       int64  `json:"created_at"`
	LastUsedAt      any    `json:"last_used_at"`
	HTTPProxyURL    string `json:"http_proxy_url"`
	HTTPSProxyURL   string `json:"https_proxy_url"`
	SOCKS5ProxyURL  string `json:"socks5_proxy_url"`
}

type CredentialList struct {
	Items            []Credential `json:"items"`
	ProxyCredentials []Credential `json:"proxy_credentials"`
	Total            int          `json:"total"`
}

func BuildCreatedCredential(input CreatedCredentialInput) CreatedCredential {
	httpURL, httpsURL, socks5URL := credentialURLs(input.ProfileIdentifier, input.Password, input.Endpoint)
	return CreatedCredential{
		ID:              input.ID,
		AccessProfileID: input.ProfileID,
		Remark:          input.Remark,
		Password:        input.Password,
		Enabled:         true,
		CreatedAt:       0,
		LastUsedAt:      nil,
		HTTPProxyURL:    httpURL,
		HTTPSProxyURL:   httpsURL,
		SOCKS5ProxyURL:  socks5URL,
	}
}

func BuildCredential(record CredentialRecord, profileIdentifier, endpoint string) Credential {
	httpURL, httpsURL, socks5URL := credentialURLs(profileIdentifier, record.Password, endpoint)
	return Credential{
		ID:              record.ID,
		AccessProfileID: record.ProfileID,
		Remark:          record.Remark,
		Password:        record.Password,
		Enabled:         record.Enabled,
		CreatedAt:       record.CreatedAt,
		LastUsedAt:      record.LastUsedAt,
		HTTPProxyURL:    httpURL,
		HTTPSProxyURL:   httpsURL,
		SOCKS5ProxyURL:  socks5URL,
	}
}

func BuildCredentials(records []CredentialRecord, profileIdentifier, endpoint string) []Credential {
	credentials := make([]Credential, 0, len(records))
	for _, record := range records {
		credentials = append(credentials, BuildCredential(record, profileIdentifier, endpoint))
	}
	return credentials
}

func BuildCredentialList(records []CredentialRecord, profileIdentifier, endpoint string) CredentialList {
	credentials := BuildCredentials(records, profileIdentifier, endpoint)
	return CredentialList{
		Items:            credentials,
		ProxyCredentials: credentials,
		Total:            len(credentials),
	}
}

func credentialURLs(identifier, password, endpoint string) (httpURL, httpsURL, socks5URL string) {
	return "http://" + identifier + ":" + password + "@" + endpoint,
		"https://" + identifier + ":" + password + "@" + endpoint,
		"socks5://" + identifier + ":" + password + "@" + endpoint
}
