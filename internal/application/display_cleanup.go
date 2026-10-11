package application

// DisplayLiveness is one authenticated native registry or identity observation.
type DisplayLiveness struct {
	RunID string
	Live  bool
}

// DisplayCensus explicitly distinguishes verified complete facts from unknown state.
type DisplayCensus struct {
	Complete bool
	Liveness []DisplayLiveness
}

// DisplayCleanupEffects supplies physical inventory and pruning under one lock.
type DisplayCleanupEffects struct {
	PruneTemporaries func() error
	Census           func() (DisplayCensus, error)
	SupportedIDs     func() ([]string, error)
	Prune            func([]string) error
}

// CleanupDisplay prunes supported annotations only after a verified complete census.
// Unknown census or inventory retains annotations; temporary cleanup remains independent.
func CleanupDisplay(effects DisplayCleanupEffects) error {
	if err := effects.PruneTemporaries(); err != nil {
		return err
	}
	stale, complete := staleDisplayIDs(effects)
	if !complete {
		return nil
	}
	return effects.Prune(stale)
}

// Unknown inventory is a conservative retention decision, not a pruning failure.
func staleDisplayIDs(effects DisplayCleanupEffects) ([]string, bool) {
	census, err := effects.Census()
	if err != nil || !census.Complete {
		return nil, false
	}
	ids, err := effects.SupportedIDs()
	if err != nil {
		return nil, false
	}
	live := make(map[string]bool, len(census.Liveness))
	for _, fact := range census.Liveness {
		live[fact.RunID] = live[fact.RunID] || fact.Live
	}
	var stale []string
	for _, id := range ids {
		if !live[id] {
			stale = append(stale, id)
		}
	}
	return stale, true
}
