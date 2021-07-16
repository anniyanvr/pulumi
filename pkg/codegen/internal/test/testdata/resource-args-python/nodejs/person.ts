// *** WARNING: this file was generated by test. ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as pulumi from "@pulumi/pulumi";
import { input as inputs, output as outputs } from "./types";
import * as utilities from "./utilities";

export class Person extends pulumi.CustomResource {
    /**
     * Get an existing Person resource's state with the given name, ID, and optional extra
     * properties used to qualify the lookup.
     *
     * @param name The _unique_ name of the resulting resource.
     * @param id The _unique_ provider ID of the resource to lookup.
     * @param opts Optional settings to control the behavior of the CustomResource.
     */
    public static get(name: string, id: pulumi.Input<pulumi.ID>, opts?: pulumi.CustomResourceOptions): Person {
        return new Person(name, undefined as any, { ...opts, id: id });
    }

    /** @internal */
    public static readonly __pulumiType = 'example::Person';

    /**
     * Returns true if the given object is an instance of Person.  This is designed to work even
     * when multiple copies of the Pulumi SDK have been loaded into the same process.
     */
    public static isInstance(obj: any): obj is Person {
        if (obj === undefined || obj === null) {
            return false;
        }
        return obj['__pulumiType'] === Person.__pulumiType;
    }

    public readonly name!: pulumi.Output<string | undefined>;
    public readonly pets!: pulumi.Output<outputs.Pet[] | undefined>;

    /**
     * Create a Person resource with the given unique name, arguments, and options.
     *
     * @param name The _unique_ name of the resource.
     * @param args The arguments to use to populate this resource's properties.
     * @param opts A bag of options that control this resource's behavior.
     */
    constructor(name: string, args?: PersonArgs, opts?: pulumi.CustomResourceOptions) {
        let inputs: pulumi.Inputs = {};
        opts = opts || {};
        if (!opts.id) {
            inputs["name"] = args ? args.name : undefined;
            inputs["pets"] = args ? args.pets : undefined;
        } else {
            inputs["name"] = undefined /*out*/;
            inputs["pets"] = undefined /*out*/;
        }
        if (!opts.version) {
            opts = pulumi.mergeOptions(opts, { version: utilities.getVersion()});
        }
        super(Person.__pulumiType, name, inputs, opts);
    }
}

/**
 * The set of arguments for constructing a Person resource.
 */
export interface PersonArgs {
    name?: pulumi.Input<string>;
    pets?: pulumi.Input<pulumi.Input<inputs.PetArgs>[]>;
}
