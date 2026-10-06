package support

// Files and a symbol the entries below name more than once.
const (
	cliCommands      = "internal/cli/commands.go"
	evidenceHistory  = "internal/evidence/history.go"
	guardEvidence    = "internal/guard/evidence.go"
	guardReservation = "internal/guard/reservation.go"
	promotionSummary = "healthyPromotionSummary"
)

// Units that remove the entries below, named in the Unit field.
const (
	removedByOutcome  = "Unit 2: typed run outcome"
	removedByLineage  = "Unit 4: profile lineage"
	removedByDecoding = "Unit 6: strict decoding"
)

// domainLiteralAllowlist is the ratchet: every violation the domain literal
// analysis finds today, so a new one fails from the day the analysis turns on.
// Each entry names the unit that removes it, and the entries sit in one group
// per unit. The list can only shrink: an entry whose violation is gone fails
// the analysis until the entry is deleted.
var domainLiteralAllowlist = []domainAllowance{
	// Unit 2 types the run outcome and the budget outcome: the evidence writer and summary, the history reader, and
	// the history command's outcome option.
	{Path: cliCommands, Symbol: "historyOptions.outcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: "Summary.Outcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: "Summary.BudgetOutcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: "Query.Outcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: promotionSummary, Rule: ruleLiteralComparison, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: promotionSummary, Rule: ruleLiteralComparison, Unit: removedByOutcome},
	{Path: evidenceHistory, Symbol: promotionSummary, Rule: ruleLiteralComparison, Unit: removedByOutcome},
	{Path: guardEvidence, Symbol: "EvidenceWriter.SetReservationContext.outcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: guardEvidence, Symbol: "EvidenceSummary.Outcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: guardEvidence, Symbol: "EvidenceSummary.BudgetOutcome", Rule: ruleRawField, Unit: removedByOutcome},
	{Path: guardEvidence, Symbol: "EvidenceWriter.Finalize.outcome", Rule: ruleRawField, Unit: removedByOutcome},

	// Unit 4 keys profile rules on lineage and types every profile name, including the owner-share switch.
	{Path: cliCommands, Symbol: "configOptions.requestedProfile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: "internal/cli/development.go", Symbol: "writeMonitorTransition.Profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardEvidence, Symbol: "EvidenceSummary.RequestedProfile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardEvidence, Symbol: "EvidenceSummary.ResolvedProfile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: "internal/guard/owner_metadata.go", Symbol: "ReservationEntry.Profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: "internal/guard/owner_metadata.go", Symbol: "reservationEntry.profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "ReservationOwner.Profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "reservationWaiter.Profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "PlanReservation", Rule: ruleLiteralComparison, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "PlanReservation", Rule: ruleLiteralComparison, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "AcquireReservation.profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: guardReservation, Symbol: "AcquireReservationWithOptions.profile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: "internal/policy/profiles.go", Symbol: "Resolution.RequestedProfile", Rule: ruleRawField, Unit: removedByLineage},
	{Path: "internal/policy/profiles.go", Symbol: "Resolution.ResolvedProfile", Rule: ruleRawField, Unit: removedByLineage},

	// Unit 6 decodes the task class strictly where it decides and tolerantly where it records: the class fields and
	// the command-line class options.
	{Path: cliCommands, Symbol: "historyOptions.taskClass", Rule: ruleRawField, Unit: removedByDecoding},
	{Path: cliCommands, Symbol: "runOptions.class", Rule: ruleRawField, Unit: removedByDecoding},
	{Path: evidenceHistory, Symbol: "Summary.TaskClass", Rule: ruleRawField, Unit: removedByDecoding},
	{Path: evidenceHistory, Symbol: "Query.Class", Rule: ruleRawField, Unit: removedByDecoding},
	{Path: guardEvidence, Symbol: "EvidenceSummary.TaskClass", Rule: ruleRawField, Unit: removedByDecoding},
	{Path: "internal/guard/lease.go", Symbol: "leaseOwner.Class", Rule: ruleRawField, Unit: removedByDecoding},
}
