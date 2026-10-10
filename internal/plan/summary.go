package plan

// RowEstimate is an entity's row count: exact for Count entities, a range
// for driven_by entities (Expected assumes uniform child counts).
type RowEstimate struct {
	Min      int64   `json:"min"`
	Max      int64   `json:"max"`
	Expected float64 `json:"expected"`
}

// EntitySummary describes what generating an entity will produce.
type EntitySummary struct {
	Name    string      `json:"name"`
	Rows    RowEstimate `json:"rows"`
	Columns []string    `json:"columns"`
}

// Summarize estimates each entity's rows and lists its columns in output
// order: driven_by injections (As, Expose, IndexAs), then declared fields.
// The plan must be valid.
func Summarize(p *Plan) []EntitySummary {
	rows := make(map[string]RowEstimate, len(p.Entities))
	out := make([]EntitySummary, len(p.Entities))

	for i := range p.Entities {
		e := &p.Entities[i]
		est := RowEstimate{Min: e.Count, Max: e.Count, Expected: float64(e.Count)}
		var cols []string

		if db := e.DrivenBy; db != nil {
			parent := rows[db.Entity]
			est = RowEstimate{
				Min:      parent.Min * db.Min,
				Max:      parent.Max * db.Max,
				Expected: parent.Expected * float64(db.Min+db.Max) / 2,
			}
			cols = append(cols, db.As)
			for _, pf := range db.Expose {
				cols = append(cols, pf.ChildName())
			}
			if db.IndexAs != "" {
				cols = append(cols, db.IndexAs)
			}
		}
		for _, f := range e.Fields {
			cols = append(cols, f.Name)
		}

		rows[e.Name] = est
		out[i] = EntitySummary{Name: e.Name, Rows: est, Columns: cols}
	}
	return out
}
