package api

import (
	"context"
	"iter"
	"net/url"
)

type PipeStore interface {
	Load(context.Context, NameDigest) (*Pipe, error)
	Store(context.Context, *Pipe) error
	StoreBatch(context.Context, []*Pipe) error
	ScanNames(context.Context) iter.Seq2[NameDigest, error]
	Scan(context.Context) iter.Seq2[*Pipe, error]
	ScanDependencies(context.Context, NameDigest) iter.Seq2[*Pipe, error]
}

type CriteriaStore interface {
	Disable(context.Context, NameDigest) error
	Store(context.Context, *TriggerCriteria) error
	ScanMatched(ctx context.Context, u *url.URL) iter.Seq2[*Pipe, error]
}

type PlanStore interface {
	Store(ctx context.Context, uuid string, plan *TriggerPlan) error
	Finish(ctx context.Context, uuid string) error
}

type ResultStore interface {
	StoreResults(ctx context.Context, trigger *TriggerEvent, results []TriggerResult) error
}

type ResourceStore interface {
	Load(context.Context, NameDigest) (*Resource, error)
	Delete(context.Context, NameDigest) error
	Store(context.Context, *Resource) error
	Scan(context.Context) iter.Seq2[*Resource, error]
}

type ViewStore interface {
	Store(ctx context.Context, uuid string, view *View) error
}
