// Package engine exposes the cold-cli engine to hosted wrappers without
// exposing the repository's internal package layout.
package engine

import (
	"database/sql"

	"github.com/andersmyrmel/cold-cli/internal"
)

type Account = internal.Account
type AccountVerifyResult = internal.AccountVerifyResult
type BackfillEmailMessagesConfig = internal.BackfillEmailMessagesConfig
type BackfillEmailMessagesResult = internal.BackfillEmailMessagesResult
type CreateCampaignOpts = internal.CreateCampaignOpts
type CreateDraftCampaignOpts = internal.CreateDraftCampaignOpts
type CloneCampaignOpts = internal.CloneCampaignOpts
type AddSMTPIMAPAccountOpts = internal.AddSMTPIMAPAccountOpts
type AddSMTPIMAPAccountResult = internal.AddSMTPIMAPAccountResult
type CreateCampaignResult = internal.CreateCampaignResult
type EmailMessage = internal.EmailMessage
type GWSClient = internal.GWSClient
type ListEmailThreadMessagesOpts = internal.ListEmailThreadMessagesOpts
type PauseAccountResult = internal.PauseAccountResult
type PreviewInboxReplyConfig = internal.PreviewInboxReplyConfig
type InboxReplyPreview = internal.InboxReplyPreview
type RemoveAccountResult = internal.PauseAccountResult
type ResumeAccountResult = internal.ResumeAccountResult
type SecretResolver = internal.SecretResolver
type SecretResolverFunc = internal.SecretResolverFunc
type SendInboxReplyConfig = internal.SendInboxReplyConfig
type SendInboxReplyResult = internal.SendInboxReplyResult
type SyncEmailThreadConfig = internal.SyncEmailThreadConfig
type SyncEmailThreadResult = internal.SyncEmailThreadResult
type SendSMTPTestEmailOpts = internal.SendSMTPTestEmailOpts
type SendSMTPTestEmailResult = internal.SendSMTPTestEmailResult
type Store = internal.Store
type TickConfig = internal.TickConfig
type TickResult = internal.TickResult
type UpdateAccountOpts = internal.UpdateAccountOpts
type UpdateCampaignOpts = internal.UpdateCampaignOpts
type UpdateSMTPIMAPAccountOpts = internal.UpdateSMTPIMAPAccountOpts

const (
	AccountProviderGWS      = internal.AccountProviderGWS
	AccountProviderSMTPIMAP = internal.AccountProviderSMTPIMAP
)

func OpenStore() (*Store, error) {
	return internal.OpenStore()
}

func AddSMTPIMAPAccount(db *sql.DB, opts AddSMTPIMAPAccountOpts) (*AddSMTPIMAPAccountResult, error) {
	return internal.AddSMTPIMAPAccount(db, opts)
}

func BackfillEmailMessages(cfg BackfillEmailMessagesConfig) (*BackfillEmailMessagesResult, error) {
	return internal.BackfillEmailMessages(cfg)
}

func UpdateSMTPIMAPAccount(db *sql.DB, email string, opts UpdateSMTPIMAPAccountOpts) (*AddSMTPIMAPAccountResult, error) {
	return internal.UpdateSMTPIMAPAccount(db, email, opts)
}

func GetAccountByEmail(db *sql.DB, email string) (Account, error) {
	return internal.GetAccountByEmail(db, email)
}

func CreateDraftCampaign(db *sql.DB, opts CreateDraftCampaignOpts) (*CreateCampaignResult, error) {
	return internal.CreateDraftCampaign(db, opts)
}

func CreateCampaign(db *sql.DB, opts CreateCampaignOpts) (*CreateCampaignResult, error) {
	return internal.CreateCampaign(db, opts)
}

func CloneCampaign(db *sql.DB, opts CloneCampaignOpts) (*CreateCampaignResult, error) {
	return internal.CloneCampaign(db, opts)
}

func UpdateCampaign(db *sql.DB, name string, opts UpdateCampaignOpts) error {
	return internal.UpdateCampaign(db, name, opts)
}

func PauseAccount(db *sql.DB, email string) (*PauseAccountResult, error) {
	return internal.PauseAccount(db, email)
}

func ResumeAccount(db *sql.DB, email string) (*ResumeAccountResult, error) {
	return internal.ResumeAccount(db, email)
}

func RemoveAccount(db *sql.DB, email string) (*RemoveAccountResult, error) {
	return internal.RemoveAccount(db, email)
}

func UpdateAccount(db *sql.DB, email string, opts UpdateAccountOpts) error {
	return internal.UpdateAccount(db, email, opts)
}

func VerifySMTPIMAPAccount(account Account, resolver SecretResolver) (*AccountVerifyResult, error) {
	return internal.VerifySMTPIMAPAccount(
		account,
		internal.NewSMTPTransport(resolver),
		internal.NewIMAPTransport(resolver),
	)
}

func SendSMTPTestEmail(account Account, opts SendSMTPTestEmailOpts, resolver SecretResolver) (*SendSMTPTestEmailResult, error) {
	return internal.SendSMTPTestEmail(account, opts, resolver)
}

func Tick(cfg TickConfig) (*TickResult, error) {
	return internal.Tick(cfg)
}

func ListEmailThreadMessages(db *sql.DB, opts ListEmailThreadMessagesOpts) ([]EmailMessage, error) {
	return internal.ListEmailThreadMessages(db, opts)
}

func SyncEmailThread(cfg SyncEmailThreadConfig) (*SyncEmailThreadResult, error) {
	return internal.SyncEmailThread(cfg)
}

func ConfiguredGWSClient(db *sql.DB) GWSClient {
	return internal.ConfiguredGWSClient(db)
}

func SendInboxReply(cfg SendInboxReplyConfig) (*SendInboxReplyResult, error) {
	return internal.SendInboxReply(cfg)
}

func PreviewInboxReply(cfg PreviewInboxReplyConfig) (*InboxReplyPreview, error) {
	return internal.PreviewInboxReply(cfg)
}

func CampaignStateTransition(db *sql.DB, name, action, fromStatus, toStatus string) error {
	return internal.CampaignStateTransition(db, name, action, fromStatus, toStatus)
}

func ResolveCampaignNameInWorkspace(db *sql.DB, workspaceID, nameOrID string) (string, error) {
	return internal.ResolveCampaignNameInWorkspace(db, workspaceID, nameOrID)
}

func GetCampaignPreview(db *sql.DB, name string) (campaignID int64, status string, preview []internal.PreviewRow, err error) {
	return internal.GetCampaignPreview(db, name)
}

func GetCampaignRenderedPreview(db *sql.DB, name string, leadEmail string) ([]internal.RenderedEmail, error) {
	return internal.GetCampaignRenderedPreview(db, name, leadEmail)
}

func GetCampaignStatus(db *sql.DB, name string) (*internal.CampaignStatusInfo, error) {
	return internal.GetCampaignStatus(db, name)
}

func AddLeadsToCampaign(db *sql.DB, campaignName, leadsFile, leadsInline string) (*internal.AddLeadsResult, error) {
	return internal.AddLeadsToCampaign(db, campaignName, leadsFile, leadsInline)
}

func RemoveLeadFromCampaign(db *sql.DB, campaignName, email string) (*internal.RemoveLeadResult, error) {
	return internal.RemoveLeadFromCampaign(db, campaignName, email)
}

func BlacklistLead(db *sql.DB, target string) (*internal.BlacklistResult, error) {
	return internal.BlacklistLead(db, target)
}

func PauseLead(db *sql.DB, email string) (*internal.PauseLeadResult, error) {
	return internal.PauseLead(db, email)
}

func ListLeads(db *sql.DB, domain, status string, limit int) ([]internal.LeadListRow, error) {
	return internal.ListLeads(db, domain, status, limit)
}

func GetCampaignStepStats(db *sql.DB, campaignID int64) ([]internal.StepStats, error) {
	return internal.GetCampaignStepStats(db, campaignID)
}

func GetCampaignVariantStats(db *sql.DB, campaignID int64) ([]internal.VariantStats, error) {
	return internal.GetCampaignVariantStats(db, campaignID)
}

func GetCampaignLeadStats(db *sql.DB, campaignID int64) ([]internal.LeadStatsRow, error) {
	return internal.GetCampaignLeadStats(db, campaignID)
}

func AddAccountInWorkspace(db *sql.DB, workspaceID, email string, dailyLimit int, configDir string) (*internal.AddAccountResult, error) {
	return internal.AddAccountInWorkspace(db, workspaceID, email, dailyLimit, configDir)
}

func NormalizeWorkspaceID(workspaceID string) string {
	return internal.NormalizeWorkspaceID(workspaceID)
}
