package store

// Store is an interface for managing repositories, users, and settings.
type Store interface {
	RepositoryStore
	UserStore
	CollaboratorStore
	BranchCollabStore
	ProtectedBranchStore
	SettingStore
	LFSStore
	AccessTokenStore
	WebhookStore
}
