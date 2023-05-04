
<div align="center">
  <br />
  <br />
  <a href="https://optimism.io"><img alt="Optimism" src="https://raw.githubusercontent.com/ethereum-optimism/brand-kit/main/assets/svg/OPTIMISM-R.svg" width=600></a>
  <br />
  <h3><a href="https://optimism.io">Optimism</a> is a low-cost and lightning-fast Ethereum L2 blockchain.</h3>
  <br />
</div>

## Milkomeda to OP stack migration WIP notes

### What?

[Here's](https://github.com/dcSpark/milkomeda-rollup/) Milkomeda the EVM rollup over Algorand. We want to create Milkomeda rollup v2 by forking the codebase of the Optimism rollup.

### Why?

The Milkomeda rollup is unique in that it sits atop of Algorand. For any other rollup features unrelated to the fact that we have Algorand as the Data Availability layer we're better off copying them from existing mature rollup implementations.

Optimism is one of the biggest existing rollups and lately they decided to direct their codebase development into a modular rollup building framework called the OP stack. The intent is exactly to make it possible to build a rollup from existing blocks and only need to provide your unique stuff.

That's theory but real life is as always a bit behind and much more messy. As of now Optimism docs (rightly) [mention](https://stack.optimism.io/docs/build/data-avail/#) that hacking into the DA layer is a messy business the codebase is not expecting, e.g. changes need to be done at lots of places and it really doesn't feel "modular". When picking up this development always keep an eye on what's been done upstream in the meantime.

### Goals a bit more concrete

By reusing Optimism code in the derivation layer and elsewhere we should be able to increase performance and reliability of Milkomeda rollup v1, however, that's not a pressing issue until it has more users.

The main first goal is to be able to plug in a settlement layer, namely Optimistic fault proofs. Milkomeda rollup v1 doesn't have any succint proofs of its current state and thus doesn't enable light clients. Users need to replay all L2 txns from DA layer to get trustless state view.

A settlement layer needs massive support on L1 and we need to rebuild this part completely as Algorand is very different from Ethereum. That's why we're also in touch with Algorand inc. for their assistance.

### Optimism architecture as related to our DA swap

Note that we're almost exclusively interested in the `op-*` folders of this repo. They contain the code for the  [Bedrock release](https://community.optimism.io/docs/developers/bedrock/explainer/#) of Optimism. Bedrock is the name of the first ever official release of the OP Stack. The other code is and will be getting removed.

#### Data Availability layer

DA layer is unfortunately not materialized in a modular way in the codebase. L1 interactions happen from a number of places (`op-batcher` which sends L2 data to L1, `op-proposer` which sends _special_ L2 data to L1 -- the state roots, `op-node` which derives L2 chain from data found on L1, `op-program` which enables fault proof resolution on L1...) and currently it pretty hardcoded everywhere that L1 is Ethereum. That's the major reason why this migration is not smooth.

#### Batcher (`op-batcher`)

Makes L2 blocks available on L1. We need it to put data on Algorand instead of Ethereum -- first into Algorand transaction notes (which is what's currently done on Milkomeda rollup v1), after we have a settlement layer we can start putting it into Algorand boxes and not keeping all data forever. It talks to both the sequencer node (`op-node`) and the execution engine (`op-geth`) which makes it a bit more difficult to test in isolation.

#### Derivation layer (`op-node`)

[The rollup node](https://github.com/ethereum-optimism/optimistic-specs/blob/main/specs/rollup-node.md) is the component responsible for deriving the L2 chain from data found on L1. Found L2 blocks are passed to the execution engine. We need to be reading the raw data off Algorand instead of Ethereum.

#### Execution engine

[The execution engine](https://github.com/ethereum-optimism/optimistic-specs/blob/main/specs/exec-engine.md) for Optimism is `op-geth`, living in a separate [repo](https://github.com/ethereum-optimism/op-geth). It's geth with few changes on top mostly accounting for a few Optimism specific chain derivation rules.

In the past we have tried plugging op-geth into Milkomeda rollup v1 [here](https://github.com/dcSpark/milkomeda-rollup/pull/108).

#### L1 - L2 bridging (`packages/contracts-bedrock/contracts/{L1,L2}`)

Optimism's bridging heavily depends on L1 smart contracts, these are of course Ethereum smart contracts and as such we can't use them on Algorand.

It will be simplest to continue using the [Milkomeda bridge](https://github.com/dcSpark/milkomeda-validator/) which we already use for bridging between L1==Algorand and the A1 Milkomeda rollup. Long-term we want to get rid of validator operated bridges though.

Note that L1->L2 transactions have a special role in the derivation layer (`op-node`) and we'll need to switch these expectations off if we don't use Optimism's bridging.

#### Settlement layer (`op-proposer`, `op-program`)

Optimism intends to use the optimistic [Cannon](https://medium.com/ethereum-optimism/cannon-cannon-cannon-introducing-cannon-4ce0d9245a03) fault proof system for its settlement layer. As of now (May 23) the goal is to release Bedrock (which does _not_ have Cannon as its settlement layer) and make sure everything's peachy and then do Cannon in the next milestone. Cannon fault proof system proof of concept / prototype lives in a [separate repo](https://github.com/ethereum-optimism/cannon), but incursions into the main monorepo are starting to happen in `op-program`.

[The proposer](https://github.com/ethereum-optimism/optimistic-specs/blob/main/specs/proposals.md) (`op-proposer`) is a service which publishes L2 state roots to L1, whatever the proof system. Unlike in Milkomeda rollup v1, state root posting is decoupled from batch posting. In Optimism the state roots are posted to a special Ethereum contract, here we of course need to make the roots available on Algorand and later be read from Algorand.

### Progress status

Search for `MILKOMEDA TO OP-STACK MIGRATION NOTES` to find detailed technical comments on the work in progress, as well as comments and TODOs in code.

Progress so far has been done in the following components:

* `milk-e2e` e2e tests to sanity check development over a real Algorand network. [Details.](https://github.com/dcSpark/optimism/blob/6810e6449d435330e4ef96320e3d2abe4998004b/milk-e2e/setup.go#L1-L27)
* `op-service` houses L1 signing and transaction manager compoments. Should be ready to get used by now.
* `op-batcher` the full batcher component, in progress

Original upstream Optimism README follows.

---

## What is Optimism?

Optimism is a low-cost and lightning-fast Ethereum L2 blockchain, **but it's also so much more than that**.

Optimism is the technical foundation for [the Optimism Collective](https://app.optimism.io/announcement), a band of communities, companies, and citizens united by a mutually beneficial pact to adhere to the axiom of **impact=profit** — the principle that positive impact to the collective should be rewarded with profit to the individual.
We're trying to solve some of the most critical coordination failures facing the crypto ecosystem today.
**We're particularly focused on creating a sustainable funding stream for the public goods and infrastructure upon which the ecosystem so heavily relies but has so far been unable to adequately reward.**
We'd love for you to check out [The Optimistic Vision](https://www.optimism.io/vision) to understand more about why we do what we do.

## Documentation

If you want to build on top of Optimism, take a look at the extensive documentation on the [Optimism Community Hub](http://community.optimism.io/).
If you want to build Optimism, check out the [Protocol Specs](./specs/).

## Community

General discussion happens most frequently on the [Optimism discord](https://discord.gg/optimism).
Governance discussion can also be found on the [Optimism Governance Forum](https://gov.optimism.io/).

## Contributing

Read through [CONTRIBUTING.md](./CONTRIBUTING.md) for a general overview of our contribution process.
Use the [Developer Quick Start](./CONTRIBUTING.md#development-quick-start) to get your development environment set up to start working on the Optimism Monorepo.
Then check out our list of [good first issues](https://github.com/ethereum-optimism/optimism/contribute) to find something fun to work on!

## Security Policy and Vulnerability Reporting

Please refer to our canonical [Security Policy](https://github.com/ethereum-optimism/.github/blob/master/SECURITY.md) document for detailed information about how to report vulnerabilities in this codebase.
Bounty hunters are encouraged to check out [our Immunefi bug bounty program](https://immunefi.com/bounty/optimism/).
We offer up to $2,000,042 for in-scope critical vulnerabilities and [we pay our maximum bug bounty rewards](https://medium.com/ethereum-optimism/disclosure-fixing-a-critical-bug-in-optimisms-geth-fork-a836ebdf7c94).

## The Bedrock Upgrade

Optimism is currently preparing for [its next major upgrade called Bedrock](https://dev.optimism.io/introducing-optimism-bedrock/).
Bedrock significantly revamps how Optimism works under the hood and will help make Optimism the fastest, cheapest, and most reliable rollup yet.
You can find detailed specifications for the Bedrock upgrade within the [specs folder](./specs) in this repository.

Please note that a significant number of packages and folders within this repository are part of the Bedrock upgrade and are NOT currently running in production.
Refer to the Directory Structure section below to understand which packages are currently running in production and which are intended for use as part of the Bedrock upgrade.

## Directory Structure

<pre>
~~ Production ~~
├── <a href="./packages">packages</a>
│   ├── <a href="./packages/common-ts">common-ts</a>: Common tools for building apps in TypeScript
│   ├── <a href="./packages/contracts">contracts</a>: L1 and L2 smart contracts for Optimism
│   ├── <a href="./packages/contracts-periphery">contracts-periphery</a>: Peripheral contracts for Optimism
│   ├── <a href="./packages/core-utils">core-utils</a>: Low-level utilities that make building Optimism easier
│   ├── <a href="./packages/data-transport-layer">data-transport-layer</a>: Service for indexing Optimism-related L1 data
│   ├── <a href="./packages/chain-mon">chain-mon</a>: Chain monitoring services
│   ├── <a href="./packages/fault-detector">fault-detector</a>: Service for detecting Sequencer faults
│   ├── <a href="./packages/message-relayer">message-relayer</a>: Tool for automatically relaying L1<>L2 messages in development
│   ├── <a href="./packages/replica-healthcheck">replica-healthcheck</a>: Service for monitoring the health of a replica node
│   └── <a href="./packages/sdk">sdk</a>: provides a set of tools for interacting with Optimism
├── <a href="./batch-submitter">batch-submitter</a>: Service for submitting batches of transactions and results to L1
├── <a href="./bss-core">bss-core</a>: Core batch-submitter logic and utilities
├── <a href="./gas-oracle">gas-oracle</a>: Service for updating L1 gas prices on L2
├── <a href="./indexer">indexer</a>: indexes and syncs transactions
├── <a href="./infra/op-replica">infra/op-replica</a>: Deployment examples and resources for running an Optimism replica
├── <a href="./integration-tests">integration-tests</a>: Various integration tests for the Optimism network
├── <a href="./l2geth">l2geth</a>: Optimism client software, a fork of <a href="https://github.com/ethereum/go-ethereum/tree/v1.9.10">geth v1.9.10</a>  (deprecated for BEDROCK upgrade)
├── <a href="./l2geth-exporter">l2geth-exporter</a>: A prometheus exporter to collect/serve metrics from an L2 geth node
├── <a href="./op-exporter">op-exporter</a>: A prometheus exporter to collect/serve metrics from an Optimism node
├── <a href="./proxyd">proxyd</a>: Configurable RPC request router and proxy
├── <a href="./technical-documents">technical-documents</a>: audits and post-mortem documents

~~ BEDROCK upgrade - Not production-ready yet, part of next major upgrade ~~
├── <a href="./packages">packages</a>
│   └── <a href="./packages/contracts-bedrock">contracts-bedrock</a>: Bedrock smart contracts. To be merged with ./packages/contracts.
├── <a href="./op-bindings">op-bindings</a>: Go bindings for Bedrock smart contracts.
├── <a href="./op-batcher">op-batcher</a>: L2-Batch Submitter, submits bundles of batches to L1
├── <a href="./op-e2e">op-e2e</a>: End-to-End testing of all bedrock components in Go
├── <a href="./op-node">op-node</a>: rollup consensus-layer client.
├── <a href="./op-proposer">op-proposer</a>: L2-Output Submitter, submits proposals to L1
├── <a href="./ops-bedrock">ops-bedrock</a>: Bedrock devnet work
└── <a href="./specs">specs</a>: Specs of the rollup starting at the Bedrock upgrade
</pre>

## Branching Model

### Active Branches

| Branch          | Status                                                                           |
| --------------- | -------------------------------------------------------------------------------- |
| [master](https://github.com/ethereum-optimism/optimism/tree/master/)                   | Accepts PRs from `develop` when we intend to deploy to mainnet.                                      |
| [develop](https://github.com/ethereum-optimism/optimism/tree/develop/)                 | Accepts PRs that are compatible with `master` OR from `release/X.X.X` branches.                    |
| release/X.X.X                                                                          | Accepts PRs for all changes, particularly those not backwards compatible with `develop` and `master`. |

### Overview

We generally follow [this Git branching model](https://nvie.com/posts/a-successful-git-branching-model/).
Please read the linked post if you're planning to make frequent PRs into this repository (e.g., people working at/with Optimism).

### Production branch

Our production branch is `master`.
The `master` branch contains the code for our latest "stable" releases.
Updates from `master` **always** come from the `develop` branch.
We only ever update the `master` branch when we intend to deploy code within the `develop` to the Optimism mainnet.
Our update process takes the form of a PR merging the `develop` branch into the `master` branch.

### Development branch

Our primary development branch is [`develop`](https://github.com/ethereum-optimism/optimism/tree/develop/).
`develop` contains the most up-to-date software that remains backwards compatible with our latest experimental [network deployments](https://community.optimism.io/docs/useful-tools/networks/).
If you're making a backwards compatible change, please direct your pull request towards `develop`.

**Changes to contracts within `packages/contracts/contracts` are usually NOT considered backwards compatible and SHOULD be made against a release candidate branch**.
Some exceptions to this rule exist for cases in which we absolutely must deploy some new contract after a release candidate branch has already been fully deployed.
If you're changing or adding a contract and you're unsure about which branch to make a PR into, default to using the latest release candidate branch.
See below for info about release candidate branches.

### Release candidate branches

Branches marked `release/X.X.X` are **release candidate branches**.
Changes that are not backwards compatible and all changes to contracts within `packages/contracts/contracts` MUST be directed towards a release candidate branch.
Release candidates are merged into `develop` and then into `master` once they've been fully deployed.
We may sometimes have more than one active `release/X.X.X` branch if we're in the middle of a deployment.
See table in the **Active Branches** section above to find the right branch to target.

## Releases

### Changesets

We use [changesets](https://github.com/changesets/changesets) to mark packages for new releases.
When merging commits to the `develop` branch you MUST include a changeset file if your change would require that a new version of a package be released.

To add a changeset, run the command `yarn changeset` in the root of this monorepo.
You will be presented with a small prompt to select the packages to be released, the scope of the release (major, minor, or patch), and the reason for the release.
Comments within changeset files will be automatically included in the changelog of the package.

### Triggering Releases

Releases can be triggered using the following process:

1. Create a PR that merges the `develop` branch into the `master` branch.
2. Wait for the auto-generated `Version Packages` PR to be opened (may take several minutes).
3. Change the base branch of the auto-generated `Version Packages` PR from `master` to `develop` and merge into `develop`.
4. Create a second PR to merge the `develop` branch into the `master` branch.

After merging the second PR into the `master` branch, packages will be automatically released to their respective locations according to the set of changeset files in the `develop` branch at the start of the process.
Please carry this process out exactly as listed to avoid `develop` and `master` falling out of sync.

**NOTE**: PRs containing changeset files merged into `develop` during the release process can cause issues with changesets that can require manual intervention to fix.
It's strongly recommended to avoid merging PRs into develop during an active release.

## License

Code forked from [`go-ethereum`](https://github.com/ethereum/go-ethereum) under the name [`l2geth`](https://github.com/ethereum-optimism/optimism/tree/master/l2geth) is licensed under the [GNU GPLv3](https://gist.github.com/kn9ts/cbe95340d29fc1aaeaa5dd5c059d2e60) in accordance with the [original license](https://github.com/ethereum/go-ethereum/blob/master/COPYING).

All other files within this repository are licensed under the [MIT License](https://github.com/ethereum-optimism/optimism/blob/master/LICENSE) unless stated otherwise.
