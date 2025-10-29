package metrics

import "github.com/prometheus/client_golang/prometheus"

type mOpts struct {
	namespace     *string
	name          string
	help          string
	buckets       []float64
	labels        map[string]string
	quantile      map[float64]float64
	withoutSuffix bool
}

func (opt *mOpts) GetCounterOpts() prometheus.CounterOpts {
	name := opt.name
	if !opt.withoutSuffix {
		name = opt.name + "_c"
	}
	ns := DefaultMetricsNamespace
	if opt.namespace != nil {
		ns = *opt.namespace
	}
	counterOpt := prometheus.CounterOpts{
		Namespace:   ns,
		Name:        name,
		ConstLabels: opt.labels,
	}
	if opt.help != "" {
		counterOpt.Help = opt.help + " (counters)"
	} else {
		counterOpt.Help = opt.name + " (counters)"
	}
	return counterOpt
}

func (opt *mOpts) GetHistogramOpts() prometheus.HistogramOpts {
	name := opt.name
	if !opt.withoutSuffix {
		name = opt.name + "_h"
	}
	ns := DefaultMetricsNamespace
	if opt.namespace != nil {
		ns = *opt.namespace
	}
	histogramOpt := prometheus.HistogramOpts{
		Namespace:   ns,
		Name:        name,
		ConstLabels: opt.labels,
		Buckets:     opt.buckets,
	}
	if opt.help != "" {
		histogramOpt.Help = opt.help + " (histogram)"
	} else {
		histogramOpt.Help = opt.name + " (histogram)"
	}
	return histogramOpt
}

func (opt *mOpts) GetSummaryOpts() prometheus.SummaryOpts {
	name := opt.name
	if !opt.withoutSuffix {
		name = opt.name + "_s"
	}
	ns := DefaultMetricsNamespace
	if opt.namespace != nil {
		ns = *opt.namespace
	}
	summaryOpt := prometheus.SummaryOpts{
		Namespace:   ns,
		Name:        name,
		ConstLabels: opt.labels,
		Objectives:  opt.quantile,
	}
	if opt.help != "" {
		summaryOpt.Help = opt.help + " (summary)"
	} else {
		summaryOpt.Help = opt.name + " (summary)"
	}
	return summaryOpt
}

func (opt *mOpts) GetGaugeOpts() prometheus.GaugeOpts {
	name := opt.name
	if !opt.withoutSuffix {
		name = opt.name + "_g"
	}
	ns := DefaultMetricsNamespace
	if opt.namespace != nil {
		ns = *opt.namespace
	}
	gaugeOpt := prometheus.GaugeOpts{
		Namespace:   ns,
		Name:        name,
		ConstLabels: opt.labels,
	}
	if opt.help != "" {
		gaugeOpt.Help = opt.help + " (gauge)"
	} else {
		gaugeOpt.Help = opt.name + " (gauge)"
	}
	return gaugeOpt

}

type OptsFunc func(opts *mOpts)

func WithNamespace(namespace string) OptsFunc {
	return func(opts *mOpts) {
		opts.namespace = &namespace
	}
}

func WithBuckets(buk []float64) OptsFunc {
	return func(o *mOpts) {
		o.buckets = buk
	}
}

func WithLabels(lables map[string]string) OptsFunc {
	return func(o *mOpts) {
		o.labels = lables
	}
}

func WithQuantile(quantile map[float64]float64) OptsFunc {
	return func(o *mOpts) {
		o.quantile = quantile
	}
}

func WithoutSuffix() OptsFunc {
	return func(o *mOpts) {
		o.withoutSuffix = true
	}
}
