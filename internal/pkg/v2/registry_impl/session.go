package registry_impl

import (
	"encoding/json"
	"fmt"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/dacctl/actions_impl/parsers"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/datamodel"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/registry"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/store"
)

func NewSessionRegistry(store store.Keystore) registry.SessionRegistry {
	return &sessionRegistry{store}
}

type sessionRegistry struct {
	store store.Keystore
}

func (s *sessionRegistry) GetSessionMutex(sessionName datamodel.SessionName) (store.Mutex, error) {
	sessionKey, err := getSessionKey(sessionName)
	if err != nil {
		return nil, err
	}
	lockKey := fmt.Sprintf("/lock%s", sessionKey)
	return s.store.NewMutex(lockKey)
}

func getSessionKey(sessionName datamodel.SessionName) (string, error) {
	if !parsers.IsValidName(string(sessionName)) {
		return "", fmt.Errorf("invalid session name %s", sessionName)
	}
	return fmt.Sprintf("/session/%s", sessionName), nil
}

func (s *sessionRegistry) CreateSession(session datamodel.Session) (datamodel.Session, error) {
	sessionKey, err := getSessionKey(session.Name)
	if err != nil {
		return session, err
	}

	sessionAsString, err := json.Marshal(session)
	if err != nil {
		return session, fmt.Errorf("unable to convert session to json due to: %s", err)
	}

	keyValueVersion, err := s.store.Create(sessionKey, string(sessionAsString))
	if err != nil {
		return session, fmt.Errorf("unable to create session due to: %s", err)
	}

	// Return the last modification revision
	session.Revision = keyValueVersion.ModRevision
	return session, nil
}

func (s *sessionRegistry) GetSession(sessionName datamodel.SessionName) (datamodel.Session, error) {
	panic("implement me")
}

func (s *sessionRegistry) GetAllSessions() ([]datamodel.Session, error) {
	panic("implement me")
}

func (s *sessionRegistry) UpdateSession(session datamodel.Session) (datamodel.Session, error) {
	panic("implement me")
}

func (s *sessionRegistry) DeleteSession(session datamodel.Session) error {
	panic("implement me")
}
