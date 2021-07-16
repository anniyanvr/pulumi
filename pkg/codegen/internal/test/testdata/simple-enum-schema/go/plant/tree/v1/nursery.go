// *** WARNING: this file was generated by test. ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

package v1

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Nursery struct {
	pulumi.CustomResourceState
}

// NewNursery registers a new resource with the given unique name, arguments, and options.
func NewNursery(ctx *pulumi.Context,
	name string, args *NurseryArgs, opts ...pulumi.ResourceOption) (*Nursery, error) {
	if args == nil {
		return nil, errors.New("missing one or more required arguments")
	}

	if args.Varieties == nil {
		return nil, errors.New("invalid value for required argument 'Varieties'")
	}
	var resource Nursery
	err := ctx.RegisterResource("plant:tree/v1:Nursery", name, args, &resource, opts...)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// GetNursery gets an existing Nursery resource's state with the given name, ID, and optional
// state properties that are used to uniquely qualify the lookup (nil if not required).
func GetNursery(ctx *pulumi.Context,
	name string, id pulumi.IDInput, state *NurseryState, opts ...pulumi.ResourceOption) (*Nursery, error) {
	var resource Nursery
	err := ctx.ReadResource("plant:tree/v1:Nursery", name, id, state, &resource, opts...)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// Input properties used for looking up and filtering Nursery resources.
type nurseryState struct {
}

type NurseryState struct {
}

func (NurseryState) ElementType() reflect.Type {
	return reflect.TypeOf((*nurseryState)(nil)).Elem()
}

type nurseryArgs struct {
	// The sizes of trees available
	Sizes map[string]TreeSize `pulumi:"sizes"`
	// The varieties available
	Varieties []RubberTreeVariety `pulumi:"varieties"`
}

// The set of arguments for constructing a Nursery resource.
type NurseryArgs struct {
	// The sizes of trees available
	Sizes TreeSizeMapInput
	// The varieties available
	Varieties RubberTreeVarietyArrayInput
}

func (NurseryArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*nurseryArgs)(nil)).Elem()
}

type NurseryInput interface {
	pulumi.Input

	ToNurseryOutput() NurseryOutput
	ToNurseryOutputWithContext(ctx context.Context) NurseryOutput
}

func (*Nursery) ElementType() reflect.Type {
	return reflect.TypeOf((*Nursery)(nil))
}

func (i *Nursery) ToNurseryOutput() NurseryOutput {
	return i.ToNurseryOutputWithContext(context.Background())
}

func (i *Nursery) ToNurseryOutputWithContext(ctx context.Context) NurseryOutput {
	return pulumi.ToOutputWithContext(ctx, i).(NurseryOutput)
}

type NurseryOutput struct {
	*pulumi.OutputState
}

func (NurseryOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*Nursery)(nil))
}

func (o NurseryOutput) ToNurseryOutput() NurseryOutput {
	return o
}

func (o NurseryOutput) ToNurseryOutputWithContext(ctx context.Context) NurseryOutput {
	return o
}

func init() {
	pulumi.RegisterOutputType(NurseryOutput{})
}
