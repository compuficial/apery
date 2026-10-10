package apery

import (
	"context"
	"io"
	"log/slog"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/runtime"
	"github.com/compuficial/apery/internal/writer"
)

type (
	Plan       = plan.Plan
	EntitySpec = plan.EntitySpec
	FieldSpec  = plan.FieldSpec
	DrivenBy   = plan.DrivenBy

	// ValidationError lists every problem in a plan; each Issue has a path,
	// a stable code, a message, and an optional hint.
	ValidationError = plan.ValidationError
	Issue           = plan.Issue

	Writer     = writer.Writer
	OrderedMap = writer.OrderedMap

	Option = runtime.Option

	GeneratorInfo = registry.GeneratorInfo
	ConfigKey     = registry.ConfigKey
)

func LoadPlanFile(path string) (*Plan, error) {
	return plan.LoadFile(path)
}

func ListGenerators() []GeneratorInfo {
	return registry.ListGenerators()
}

func WithLogger(logger *slog.Logger) Option {
	return runtime.WithLogger(logger)
}

func WithWorkers(n int) Option {
	return runtime.WithWorkers(n)
}

func WithChunkSize(n int64) Option {
	return runtime.WithChunkSize(n)
}

// WithRowLimit caps every entity at n rows, for previews.
func WithRowLimit(n int64) Option {
	return runtime.WithRowLimit(n)
}

// WithEntities writes only the named entities, still generating the ones
// they reference; written entities match a full run.
func WithEntities(names ...string) Option {
	return runtime.WithEntities(names...)
}

// ValidatePlan checks the plan's structure and builds every generator once,
// returning a *ValidationError that lists all problems.
func ValidatePlan(p *Plan) error {
	return runtime.Validate(p)
}

func NewJSONLWriter(path string) (*writer.JSONLWriter, error) {
	return writer.NewJSONLWriter(path)
}

func NewCSVWriter(path string) (*writer.CSVWriter, error) {
	return writer.NewCSVWriter(path)
}

func NewJSONLWriterFromWriter(w io.Writer) *writer.JSONLWriter {
	return writer.NewJSONLWriterFromWriter(w)
}

func NewCSVWriterFromWriter(w io.Writer) *writer.CSVWriter {
	return writer.NewCSVWriterFromWriter(w)
}

func NewSplitWriter(dir, format string) *writer.SplitWriter {
	return writer.NewSplitWriter(dir, format)
}

// Run validates the plan and streams generated records to w. Run takes
// ownership of w and closes it when done, whether or not generation succeeds.
func Run(ctx context.Context, p *Plan, w Writer, opts ...Option) error {
	executor := runtime.New(w, opts...)
	return executor.Run(ctx, p)
}
