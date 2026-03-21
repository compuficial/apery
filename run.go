package apery

import (
	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/runtime"
	"apery/internal/writer"
	"context"
	"io"
)

type (
	Plan       = plan.Plan
	EntitySpec = plan.EntitySpec
	FieldSpec  = plan.FieldSpec
	DrivenBy   = plan.DrivenBy

	Writer     = writer.Writer
	OrderedMap = writer.OrderedMap

	Logger = runtime.Logger
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

func WithLogger(logger Logger) Option {
	return runtime.WithLogger(logger)
}

func WithWorkers(n int) Option {
	return runtime.WithWorkers(n)
}

func WithChunkSize(n int64) Option {
	return runtime.WithChunkSize(n)
}

func ValidatePlan(p *Plan) error {
	return plan.Validate(p)
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

func Run(ctx context.Context, p *Plan, w Writer, opts ...Option) error {
	executor := runtime.New(w, opts...)
	return executor.Run(ctx, p)
}
