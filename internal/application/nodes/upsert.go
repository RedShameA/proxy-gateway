package nodes

type Service struct {
	NewNodeID func() (string, error)
}

type UpsertRepository interface {
	FindNodeIDByFingerprint(fingerprint string) (string, error)
	CreateNode(record CreateNodeRecord) error
	BindNodeSource(record BindSourceRecord) error
}

type UpsertInput struct {
	Fingerprint  string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	SourceID     string
	SourceName   string
	SourceType   string
	NowMillis    int64
}

type CreateNodeRecord struct {
	ID           string
	Fingerprint  string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	SourceID     string
	CreatedAt    int64
}

type BindSourceRecord struct {
	NodeID      string
	SourceID    string
	SourceName  string
	SourceType  string
	DisplayName string
	CreatedAt   int64
}

func (s Service) Upsert(repo UpsertRepository, input UpsertInput) (string, error) {
	id, err := repo.FindNodeIDByFingerprint(input.Fingerprint)
	if err != nil {
		return "", err
	}
	if id == "" {
		id, err = s.NewNodeID()
		if err != nil {
			return "", err
		}
		if err := repo.CreateNode(CreateNodeRecord{
			ID:           id,
			Fingerprint:  input.Fingerprint,
			Name:         input.Name,
			Type:         input.Type,
			Server:       input.Server,
			ServerPort:   input.ServerPort,
			Username:     input.Username,
			Password:     input.Password,
			RawJSON:      input.RawJSON,
			OutboundJSON: input.OutboundJSON,
			SourceID:     input.SourceID,
			CreatedAt:    input.NowMillis,
		}); err != nil {
			return "", err
		}
	}
	if err := repo.BindNodeSource(BindSourceRecord{
		NodeID:      id,
		SourceID:    input.SourceID,
		SourceName:  input.SourceName,
		SourceType:  input.SourceType,
		DisplayName: input.Name,
		CreatedAt:   input.NowMillis,
	}); err != nil {
		return "", err
	}
	return id, nil
}
