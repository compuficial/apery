package apery

import (
	"apery/internal/plan"
	"apery/internal/runtime"
	"apery/internal/writer"
	"context"
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
)

func WithLogger(logger Logger) Option {
	return runtime.WithLogger(logger)
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

func Run(ctx context.Context, p *Plan, w Writer, opts ...Option) error {
	executor := runtime.New(w, opts...)
	return executor.Run(ctx, p)
}
