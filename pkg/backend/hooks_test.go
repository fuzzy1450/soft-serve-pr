package backend

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/hooks"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/store"
)

// hookCtx returns a context enriched with db and store so that webhook-firing
// backend methods (AddCollaborator, Update, etc.) can read them via
// db.FromContext / store.FromContext without panicking.  Also embeds the given
// user as the "acting" user (proto.UserFromContext).
func hookCtx(base context.Context, be *Backend, user proto.User) context.Context {
	ctx := proto.WithUserContext(base, user)
	ctx = db.WithContext(ctx, be.db)
	ctx = store.WithContext(ctx, be.store)
	return ctx
}

func TestUpdateHook_RejectsProtectedBranchWithoutGrant(t *testing.T) {
	base, be := setupBackend(t)

	_, _ = be.CreateUser(base, "alice", proto.UserOptions{})
	owner, _ := be.CreateUser(base, "owner", proto.UserOptions{})
	if _, err := be.CreateRepository(proto.WithUserContext(base, owner), "demo", owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}

	// ownerCtx is needed for AddCollaborator's webhook path.
	ownerCtx := hookCtx(base, be, owner)
	if err := be.AddCollaborator(ownerCtx, "demo", "alice", access.ReadWriteAccess); err != nil {
		t.Fatal(err)
	}
	if err := be.ProtectBranch(base, "demo", "main"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOFT_SERVE_USERNAME", "alice")

	// Update is called with enriched context so webhook code paths don't panic;
	// however the ACL check should reject before ever reaching the webhook.
	alice, _ := be.User(base, "alice")
	updateCtx := hookCtx(base, be, alice)

	var stderr bytes.Buffer
	err := be.Update(updateCtx, nil, &stderr, "demo", hooks.HookArg{
		RefName: "refs/heads/main",
		OldSha:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NewSha:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err == nil {
		t.Fatal("expected error rejecting protected push; got nil")
	}
	if !strings.Contains(stderr.String(), "protected") {
		t.Fatalf("stderr does not mention protection: %q", stderr.String())
	}
}

func TestUpdateHook_AllowsProtectedBranchWithGrant(t *testing.T) {
	base, be := setupBackend(t)

	_, _ = be.CreateUser(base, "alice", proto.UserOptions{})
	owner, _ := be.CreateUser(base, "owner", proto.UserOptions{})
	if _, err := be.CreateRepository(proto.WithUserContext(base, owner), "demo", owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}
	// AddBranchCollab does not fire webhooks; plain ctx is fine.
	if err := be.AddBranchCollab(base, "demo", "alice", "main", access.ReadWriteAccess); err != nil {
		t.Fatal(err)
	}
	if err := be.ProtectBranch(base, "demo", "main"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOFT_SERVE_USERNAME", "alice")

	// Update needs db+store+user in ctx so the push webhook can look up the repo owner.
	alice, _ := be.User(base, "alice")
	updateCtx := hookCtx(base, be, alice)

	err := be.Update(updateCtx, nil, &bytes.Buffer{}, "demo", hooks.HookArg{
		RefName: "refs/heads/main",
		OldSha:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NewSha:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("expected accept, got error: %v", err)
	}
}

func TestUpdateHook_AllowsUnprotectedBranch(t *testing.T) {
	base, be := setupBackend(t)

	_, _ = be.CreateUser(base, "alice", proto.UserOptions{})
	owner, _ := be.CreateUser(base, "owner", proto.UserOptions{})
	if _, err := be.CreateRepository(proto.WithUserContext(base, owner), "demo", owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}

	ownerCtx := hookCtx(base, be, owner)
	if err := be.AddCollaborator(ownerCtx, "demo", "alice", access.ReadWriteAccess); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOFT_SERVE_USERNAME", "alice")

	alice, _ := be.User(base, "alice")
	updateCtx := hookCtx(base, be, alice)

	err := be.Update(updateCtx, nil, &bytes.Buffer{}, "demo", hooks.HookArg{
		RefName: "refs/heads/feature/x",
		OldSha:  "0000000000000000000000000000000000000000",
		NewSha:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("expected accept on unprotected branch, got: %v", err)
	}
}
