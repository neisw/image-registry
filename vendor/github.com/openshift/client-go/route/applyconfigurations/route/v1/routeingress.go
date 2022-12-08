// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	routev1 "github.com/openshift/api/route/v1"
)

// RouteIngressApplyConfiguration represents an declarative configuration of the RouteIngress type for use
// with apply.
type RouteIngressApplyConfiguration struct {
	Host                    *string                                   `json:"host,omitempty"`
	RouterName              *string                                   `json:"routerName,omitempty"`
	Conditions              []RouteIngressConditionApplyConfiguration `json:"conditions,omitempty"`
	WildcardPolicy          *routev1.WildcardPolicyType               `json:"wildcardPolicy,omitempty"`
	RouterCanonicalHostname *string                                   `json:"routerCanonicalHostname,omitempty"`
}

// RouteIngressApplyConfiguration constructs an declarative configuration of the RouteIngress type for use with
// apply.
func RouteIngress() *RouteIngressApplyConfiguration {
	return &RouteIngressApplyConfiguration{}
}

// WithHost sets the Host field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Host field is set to the value of the last call.
func (b *RouteIngressApplyConfiguration) WithHost(value string) *RouteIngressApplyConfiguration {
	b.Host = &value
	return b
}

// WithRouterName sets the RouterName field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the RouterName field is set to the value of the last call.
func (b *RouteIngressApplyConfiguration) WithRouterName(value string) *RouteIngressApplyConfiguration {
	b.RouterName = &value
	return b
}

// WithConditions adds the given value to the Conditions field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Conditions field.
func (b *RouteIngressApplyConfiguration) WithConditions(values ...*RouteIngressConditionApplyConfiguration) *RouteIngressApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithConditions")
		}
		b.Conditions = append(b.Conditions, *values[i])
	}
	return b
}

// WithWildcardPolicy sets the WildcardPolicy field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the WildcardPolicy field is set to the value of the last call.
func (b *RouteIngressApplyConfiguration) WithWildcardPolicy(value routev1.WildcardPolicyType) *RouteIngressApplyConfiguration {
	b.WildcardPolicy = &value
	return b
}

// WithRouterCanonicalHostname sets the RouterCanonicalHostname field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the RouterCanonicalHostname field is set to the value of the last call.
func (b *RouteIngressApplyConfiguration) WithRouterCanonicalHostname(value string) *RouteIngressApplyConfiguration {
	b.RouterCanonicalHostname = &value
	return b
}