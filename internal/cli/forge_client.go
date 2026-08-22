package cli

import (
	"github.com/snonux/gitsyncer/internal/codeberg"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/github"
)

// ForgeClientResolver is the single composition-root construction point for
// forge-specific clients, replacing two mechanisms that used to overlap
// here: a switch-based newRepoClientForOrg that only ever handed back a
// narrow forge.RepoClient, and repoClientFactory's fat 6-method interface
// (formerly in repo_client_factory.go) whose methods all just called
// github.NewClient/codeberg.NewClient again with the result narrowed to a
// different role interface. Both existed because github.Client and
// codeberg.Client already satisfy every role interface callers need
// (forge.RepoClient and forge.RepoDescriptionClient structurally; public
// repo listing via the adapters in public_repo_lister.go) - there was never
// a reason to construct a client more than once per org. ClientFor builds
// the client once and callers assign the result to whichever narrow
// interface they actually depend on. Adding a new forge means adding one
// case here, not touching a second switch and a fat interface.
type ForgeClientResolver struct{}

// forgeClientResolver is the seam callers in this package depend on instead
// of the concrete ForgeClientResolver, so tests can substitute a stub that
// returns fakes without constructing real forge clients that would try to
// hit the network.
type forgeClientResolver interface {
	ClientFor(org *config.Organization) (forge.RepoDescriptionClient, bool)
}

// ClientFor builds the forge.RepoDescriptionClient for org's configured
// forge (GitHub, Codeberg, or Forgejo/Gitea - Forgejo is Gitea-compatible
// and served by the same codeberg.Client via NewForgejoClient). The bool is
// false when org.Host does not match any supported forge.
//
// forge.RepoDescriptionClient embeds forge.RepoClient, so callers that only
// need create/delete/exists/HasToken (e.g. HandleDeleteRepo) can call those
// methods directly on the returned value without a separate constructor or
// interface.
func (ForgeClientResolver) ClientFor(org *config.Organization) (forge.RepoDescriptionClient, bool) {
	switch {
	case org.IsGitHub():
		return github.NewClient(org.GitHubToken, org.Name), true
	case org.IsCodeberg():
		return codeberg.NewClient(org.CodebergToken, org.Name), true
	case org.IsForgejo():
		return codeberg.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType), true
	default:
		return nil, false
	}
}

// newForgejoBackupClient constructs a Forgejo client for the given backup
// organization, satisfying forge.PublicRepoEnsurer. It is injected into
// sync.Syncer via SetForgejoBackupClientFactory so internal/sync can ensure
// backup repositories exist without importing the concrete codeberg package
// itself; internal/cli is the composition root that knows which concrete
// forge client to build, same as ForgeClientResolver.ClientFor above.
func newForgejoBackupClient(org *config.Organization) forge.PublicRepoEnsurer {
	return codeberg.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType)
}
