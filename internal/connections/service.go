package connections

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
)

// Timeouts for calls made while a person waits.
const (
	validateTimeout   = 10 * time.Second
	backgroundTimeout = 2 * time.Minute
	// validationFailed is the only thing an admin learns when a pasted token
	// cannot be used.
	validationFailed = "Couldn't verify that address and token. Check the address is the server's root URL and the token has the scopes listed above; details are in the server log."
)

// Service is what Settings does with connections. Every method requires the
// board's admin.
type Service struct {
	catalog *Catalog
	flows   *flows
	repos   *app.RepoService
	alerts  app.Notifier
	policy  Policy
	logger  *slog.Logger
}

// Policy holds the optional operator restrictions.
type Policy struct {
	// AllowedGitHubLogins, when set, refuses any other GitHub account.
	AllowedGitHubLogins []string
	// AllowedForgejoHosts, when set, refuses any other Forgejo host.
	AllowedForgejoHosts []string
}

// NewService wires the service.
func NewService(catalog *Catalog, repos *app.RepoService, alerts app.Notifier, policy Policy) *Service {
	return &Service{catalog: catalog, flows: newFlows(catalog.clock), repos: repos, alerts: alerts, policy: policy, logger: catalog.logger}
}

// List describes every provider for Settings.
func (s *Service) List(ctx context.Context, admin Admin) ([]View, error) {
	if err := admin.require(); err != nil {
		return nil, err
	}
	summaries, err := s.repos.Summaries(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(Providers))
	for _, provider := range Providers {
		view, err := s.view(ctx, provider, summaries[provider])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// SyncAll syncs every connected provider, then describes them.
func (s *Service) SyncAll(ctx context.Context, admin Admin) ([]View, error) {
	if err := admin.require(); err != nil {
		return nil, err
	}
	if err := s.repos.Sync(ctx); err != nil {
		return nil, err
	}
	return s.List(ctx, admin)
}

// StartGitHub begins a device sign-in.
func (s *Service) StartGitHub(ctx context.Context, admin Admin) (FlowView, error) {
	if err := admin.require(); err != nil {
		return FlowView{}, err
	}
	if !s.catalog.github.IsAvailable() {
		return FlowView{}, domain.Conflictf("GitHub sign-in needs GITHUB_APP_CLIENT_ID on the server")
	}
	if err := s.flows.reserve(admin.ID); err != nil {
		return FlowView{}, err
	}
	device, err := s.catalog.github.StartDevice(ctx)
	if err != nil {
		return FlowView{}, err
	}
	started, err := s.flows.add(admin.ID, device)
	if err != nil {
		return FlowView{}, err
	}
	return FlowView{ID: started.id, UserCode: device.UserCode, ExpiresAt: device.ExpiresAt, Interval: device.Interval}, nil
}

// PollGitHub checks a device sign-in. When GitHub says yes, it does not
// connect anything: it answers "confirm" with the account, and the admin must
// confirm that account.
func (s *Service) PollGitHub(ctx context.Context, admin Admin, flowID string) (PollView, error) {
	if err := admin.require(); err != nil {
		return PollView{}, err
	}
	current, isDue, err := s.flows.beginPoll(admin.ID, flowID)
	if err != nil {
		return PollView{}, err
	}
	if !isDue {
		return s.pollView(ctx, current, reposource.PollPending, "")
	}
	result, user, err := s.askGitHub(ctx, current)
	if err != nil {
		s.flows.abortPoll(flowID)
		return PollView{}, err
	}
	if result.Outcome == reposource.PollConnected && !s.isAllowedLogin(user.Login) {
		result.Outcome = reposource.PollDenied
		s.flows.endPoll(flowID, result, user)
		return PollView{Status: reposource.PollDenied, Interval: result.Interval,
			Message: fmt.Sprintf("@%s is not in GITHUB_ALLOWED_LOGINS on the server", user.Login)}, nil
	}
	s.flows.endPoll(flowID, result, user)
	current.state, current.user, current.interval = stateFor(result.Outcome), user, result.Interval
	return s.pollView(ctx, current, result.Outcome, "")
}

// ConfirmGitHub stores the confirmed account and starts a background sync.
func (s *Service) ConfirmGitHub(ctx context.Context, admin Admin, flowID string) (View, error) {
	if err := admin.require(); err != nil {
		return View{}, err
	}
	confirmed, err := s.flows.take(admin.ID, flowID)
	if err != nil {
		return View{}, err
	}
	record := Record{
		Provider: domain.ProviderGitHub, Account: confirmed.user.Login, AccountID: strconv.FormatInt(confirmed.user.ID, 10),
		AccessToken: confirmed.tokens.AccessToken, RefreshToken: confirmed.tokens.RefreshToken,
		AccessExpiresAt: confirmed.tokens.AccessExpiresAt, RefreshExpiresAt: confirmed.tokens.RefreshExpiresAt,
	}
	return s.connect(ctx, admin, record)
}

// CancelGitHub abandons a device sign-in.
func (s *Service) CancelGitHub(admin Admin, flowID string) error {
	if err := admin.require(); err != nil {
		return err
	}
	s.flows.cancel(admin.ID, flowID)
	return nil
}

// ConnectForgejo validates a pasted Forgejo or Gitea token and stores it.
func (s *Service) ConnectForgejo(ctx context.Context, admin Admin, baseURL, token string) (View, error) {
	if err := admin.require(); err != nil {
		return View{}, err
	}
	normalized, err := reposource.NormalizeForgejoURL(baseURL, s.policy.AllowedForgejoHosts)
	if err != nil {
		return View{}, err
	}
	secret, err := reposource.CheckToken(token)
	if err != nil {
		return View{}, err
	}
	login, err := s.validate(ctx, reposource.NewForgejo(s.catalog.http, normalized, secret))
	if err != nil {
		return View{}, err
	}
	return s.connect(ctx, admin, Record{Provider: domain.ProviderForgejo, BaseURL: normalized, Account: login, AccessToken: secret})
}

// ConnectBitbucket validates pasted Bitbucket credentials and stores them.
func (s *Service) ConnectBitbucket(ctx context.Context, admin Admin, email, token string) (View, error) {
	if err := admin.require(); err != nil {
		return View{}, err
	}
	cleanEmail, err := reposource.CheckEmail(email)
	if err != nil {
		return View{}, err
	}
	secret, err := reposource.CheckToken(token)
	if err != nil {
		return View{}, err
	}
	login, err := s.validate(ctx, reposource.NewBitbucket(s.catalog.http, cleanEmail, secret))
	if err != nil {
		return View{}, err
	}
	return s.connect(ctx, admin, Record{Provider: domain.ProviderBitbucket, Email: cleanEmail, Account: login, AccessToken: secret})
}

// Disconnect forgets a provider's stored credentials. The provider-side grant
// stays until the person revokes it there.
func (s *Service) Disconnect(ctx context.Context, admin Admin, provider domain.Provider) error {
	if err := admin.require(); err != nil {
		return err
	}
	if !slices.Contains(Providers, provider) {
		return domain.NotFoundf("unknown provider %q", provider)
	}
	s.catalog.refreshMu.Lock()
	err := s.catalog.store.Delete(ctx, provider)
	s.catalog.refreshMu.Unlock()
	if err != nil {
		return err
	}
	s.audit(admin, provider, "disconnected", "")
	return nil
}

// connect replaces whatever is stored for the provider, audits it, and starts
// a sync so the repo list fills in without another click.
func (s *Service) connect(ctx context.Context, admin Admin, record Record) (View, error) {
	if err := s.store(ctx, admin, record); err != nil {
		return View{}, err
	}
	go s.syncInBackground(record.Provider)
	summaries, err := s.repos.Summaries(ctx)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, record.Provider, summaries[record.Provider])
}

// store saves the connection under the refresh lock, so a refresh still in
// flight for the previous account cannot overwrite it afterwards.
func (s *Service) store(ctx context.Context, admin Admin, record Record) error {
	s.catalog.refreshMu.Lock()
	defer s.catalog.refreshMu.Unlock()
	sealed, isFound, err := s.catalog.store.Get(ctx, record.Provider)
	if err != nil {
		return err
	}
	expected, previous := int64(0), ""
	if isFound {
		expected, previous = sealed.Generation, sealed.Account
	}
	record.Status, record.ConnectedBy, record.ConnectedAt = StatusConnected, admin.ID, s.catalog.clock.Now()
	isSaved, err := s.catalog.save(ctx, record, expected)
	if err != nil {
		return err
	}
	if !isSaved {
		return domain.Conflictf("the connection changed while saving; try again")
	}
	s.audit(admin, record.Provider, "connected", describeReplacement(record.Account, previous))
	return nil
}

func (s *Service) syncInBackground(provider domain.Provider) {
	ctx, cancel := context.WithTimeout(context.Background(), backgroundTimeout)
	defer cancel()
	if err := s.repos.SyncProvider(ctx, provider); err != nil {
		s.logger.Error("sync after connecting failed", "provider", provider, "error", err)
	}
}

func (s *Service) validate(ctx context.Context, source interface {
	Whoami(context.Context) (string, error)
}) (string, error) {
	validateCtx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()
	login, err := source.Whoami(validateCtx)
	if err != nil {
		// The reason stays in the server log. The admin sees one fixed message,
		// so this form cannot be used to tell a closed port from a live service
		// or a 401 from a 404 elsewhere on the network.
		s.logger.Info("connection validation failed", "error", err)
		return "", domain.Invalidf("%s", validationFailed)
	}
	return login, nil
}

func (s *Service) askGitHub(ctx context.Context, current flow) (reposource.PollResult, reposource.GitHubUser, error) {
	result, err := s.catalog.github.Poll(ctx, current.device)
	if err != nil || result.Outcome != reposource.PollConnected {
		return result, reposource.GitHubUser{}, err
	}
	user, err := s.catalog.github.User(ctx, result.Tokens.AccessToken)
	return result, user, err
}

func (s *Service) pollView(ctx context.Context, current flow, outcome, message string) (PollView, error) {
	view := PollView{Status: outcome, Interval: current.interval, Message: message}
	if current.state != flowAwaitingConfirmation {
		return view, nil
	}
	view.Status, view.Login = PollConfirm, current.user.Login
	sealed, isFound, err := s.catalog.store.Get(ctx, domain.ProviderGitHub)
	if err != nil {
		return PollView{}, err
	}
	if isFound && !strings.EqualFold(sealed.Account, current.user.Login) {
		view.Replaces = sealed.Account
	}
	return view, nil
}

func (s *Service) view(ctx context.Context, provider domain.Provider, summary app.ProviderSummary) (View, error) {
	view := View{Provider: provider, Label: labels[provider], IsAvailable: true, Status: StatusDisconnected,
		RepoCount: summary.RepoCount, LastSync: summary.LastSync}
	if provider == domain.ProviderGitHub {
		view.IsAvailable = s.catalog.github.IsAvailable()
	}
	record, isStored, err := s.catalog.load(ctx, provider)
	if err != nil {
		return View{}, err
	}
	if isStored {
		view.ManagedBy, view.Status, view.Account = ManagedBySettings, record.Status, record.Account
		view.BaseURL, view.Email, view.ConnectedAt = record.BaseURL, record.Email, &record.ConnectedAt
		return view, nil
	}
	if s.catalog.isEnvConfigured(provider) {
		view.ManagedBy, view.Status = ManagedByServerEnv, StatusConnected
	}
	return view, nil
}

func (s *Service) isAllowedLogin(login string) bool {
	if len(s.policy.AllowedGitHubLogins) == 0 {
		return true
	}
	return slices.ContainsFunc(s.policy.AllowedGitHubLogins, func(allowed string) bool {
		return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(allowed), "@"), login)
	})
}

// audit logs a connection change and pushes it to the owner, since a new
// connection grants the board access to an account.
func (s *Service) audit(admin Admin, provider domain.Provider, what, detail string) {
	message := fmt.Sprintf("%s %s by %s%s", labels[provider], what, admin.Name, detail)
	s.logger.Info("connection changed", "provider", provider, "change", what, "by", admin.Name, "detail", detail)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), validateTimeout)
		defer cancel()
		notification := app.Notification{Title: "Board connection " + what, Message: message, Priority: app.PriorityDefault, Tags: []string{"key"}}
		if err := s.alerts.Notify(ctx, notification); err != nil {
			s.logger.Warn("connection audit push failed", "error", err)
		}
	}()
}

func describeReplacement(account, previous string) string {
	if previous == "" || strings.EqualFold(previous, account) {
		return fmt.Sprintf(" (@%s)", account)
	}
	return fmt.Sprintf(" (@%s, replacing @%s)", account, previous)
}

func stateFor(outcome string) flowState {
	if outcome == reposource.PollConnected {
		return flowAwaitingConfirmation
	}
	return flowWaiting
}
