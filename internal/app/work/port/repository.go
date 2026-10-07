package port

import (
	"context"
	"local/rag-project/internal/app/work/domain"
)

// Repository commits versions, current pointers and request results atomically.
// Every nested operation carries server-resolved user and topic identities.
type Repository interface {
	CreateTopic(context.Context, string, domain.CreateTopic) (domain.Topic, error)
	ListTopics(context.Context, string, string, domain.Page) ([]domain.Topic, error)
	GetTopic(context.Context, string, string) (domain.Topic, error)
	UpdateTopic(context.Context, string, string, domain.UpdateTopic) (domain.Topic, error)
	CreateItem(context.Context, string, string, domain.CreateItem) (domain.Item, error)
	ListItems(context.Context, string, string, domain.Page) ([]domain.Item, error)
	CreateConversation(context.Context, string, string, domain.CreateConversation) (domain.Conversation, error)
	ListConversations(context.Context, string, string, domain.Page) ([]domain.Conversation, error)
	GetState(context.Context, string, string) (domain.State, error)
	SaveState(context.Context, string, string, domain.SaveState) (domain.State, error)
	CreateArtifact(context.Context, string, string, domain.SaveArtifact) (domain.ArtifactDetail, error)
	ListArtifacts(context.Context, string, string, domain.Page) ([]domain.Artifact, error)
	GetArtifact(context.Context, string, string, string) (domain.ArtifactDetail, error)
	SaveArtifact(context.Context, string, string, string, domain.SaveArtifact) (domain.ArtifactDetail, error)
	ListVersions(context.Context, string, string, string, domain.Page) ([]domain.ArtifactVersionSummary, error)
	GetVersion(context.Context, string, string, string, int) (domain.ArtifactVersion, error)
	RestoreArtifact(context.Context, string, string, string, domain.RestoreArtifact) (domain.ArtifactDetail, error)
}
