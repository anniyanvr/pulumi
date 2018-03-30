// Copyright 2016-2018, Pulumi Corporation.  All rights reserved.

import * as pulumi from "@pulumi/pulumi";

let currentID = 0;

export class Provider implements pulumi.dynamic.ResourceProvider {
    public static readonly instance = new Provider();

    private inject: Error | undefined;

    public readonly diff: (id: pulumi.ID, olds: any, news: any) => Promise<pulumi.dynamic.DiffResult>;
    public readonly create: (inputs: any) => Promise<pulumi.dynamic.CreateResult>;
    public readonly update: (id: pulumi.ID, olds: any, news: any) => Promise<pulumi.dynamic.UpdateResult>;
    public readonly delete: (id: pulumi.ID, props: any) => Promise<void>;

    constructor() {
        this.diff = async (id: pulumi.ID, olds: any, news: any) => {
            let replaces: string[] = [];
            if ((olds as ResourceProps).replace !== (news as ResourceProps).replace) {
                replaces.push("replace");
            }
            return {
                replaces: replaces,
            };
        };

        this.create = async (inputs: any) => {
            if (this.inject) {
                throw this.inject;
            }
            return {
                id: (currentID++).toString(),
                outs: undefined,
            };
        };

        this.update = async (id: pulumi.ID, olds: any, news: any) => {
            if (this.inject) {
                throw this.inject;
            }
            return {};
        };

        this.delete = async (id: pulumi.ID, props: any) => {
            if (this.inject) {
                throw this.inject;
            }
        }
    }

    // injectFault instructs the provider to inject the given fault upon the next CRUD operation.  Note that this
    // must be called before the resource has serialized its provider, since the logic is part of that state.
    public injectFault(error: Error | undefined): void {
        this.inject = error;
    }
}

export class Resource extends pulumi.dynamic.Resource {
    constructor(name: string, props: ResourceProps, opts?: pulumi.ResourceOptions) {
        super(Provider.instance, name, props, opts);
    }
}

export interface ResourceProps {
    state?: any; // arbitrary state bag that can be updated without replacing.
    replace?: any; // arbitrary state bag that requires replacement when updating.
    resource?: pulumi.Resource; // to force a dependency on a resource.
}
