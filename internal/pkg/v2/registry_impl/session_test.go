package registry_impl

import (
	"errors"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/datamodel"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/mock_store"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/v2/store"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSessionRegistry_GetSessionMutex(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	keystore := mock_store.NewMockKeystore(mockCtrl)
	registry := NewSessionRegistry(keystore)
	fakeErr := errors.New("fake")
	keystore.EXPECT().NewMutex("/lock/session/foo").Return(nil, fakeErr)

	mutex, err := registry.GetSessionMutex("foo")
	assert.Nil(t, mutex)
	assert.Equal(t, fakeErr, err)

	mutex, err = registry.GetSessionMutex("foo/bar")
	assert.Nil(t, mutex)
	assert.Equal(t, "invalid session name foo/bar", err.Error())
}

func TestSessionRegistry_CreateSession(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	keystore := mock_store.NewMockKeystore(mockCtrl)
	registry := NewSessionRegistry(keystore)
	session := datamodel.Session{Name:"foo"}
	expectedValue := `{"Name":"foo","Revision":0,"Owner":0,"Group":0,"CreatedAt":0,"VolumeRequest":{"MultiJob":false,"Caller":"","TotalCapacityBytes":0,"PoolName":"","Access":0,"Type":0,"SwapBytes":0},"Status":{"Error":null,"FileSystemCreated":false,"CopyDataInComplete":false,"CopyDataOutComplete":false,"DeleteRequested":false,"DeleteSkipCopyDataOut":false},"StageInRequests":null,"StageOutRequests":null,"MultiJobAttachments":null,"Paths":null,"ActualSizeBytes":0,"Allocations":null,"PrimaryBrickHost":"","RequestedAttachHosts":null,"FilesystemStatus":{"Error":null,"InternalName":"","InternalData":""},"CurrentAttachments":null}`
	keystore.EXPECT().Create("/session/foo", expectedValue).Return(store.KeyValueVersion{ModRevision:42}, nil)

	session, err := registry.CreateSession(session)

	assert.Nil(t, err)
	assert.Equal(t, int64(42), session.Revision)
}