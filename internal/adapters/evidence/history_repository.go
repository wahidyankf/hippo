package evidence

import domain "github.com/wahidyankf/hippo/internal/domain/evidence"

// HistoryRepository reads retained summaries for observation use cases.
type HistoryRepository struct{}

// History reads matching current summaries and compacted daily archives.
func (HistoryRepository) History(root string, query domain.Query) ([]domain.Summary, error) {
	return ReadHistory(root, query)
}
