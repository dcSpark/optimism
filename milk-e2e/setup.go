/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

This package is a temporary replacement of the op-e2e package.

We can try to update op-e2e only after we have all components ready for Milkomeda.
On the other hand, as we're gradually updating them, we need to continually test
over a real Algorand network to sanity check.

In Milkomeda rollup V1 (https://github.com/dcSpark/milkomeda-rollup), we use pre-built
binaries/docker containers to start up several components in e2e testing. Here in
Optimism, as it's mostly all golang, the e2e tests instead import all source codebases
and construct high-level components to be tested. This presumably runs with more
efficiency and flexibility, but also at the expense of much more setup code and
codebase knowledge needed.

If we want reuse op-e2e, it would be most natural to continue the latter approach.
Algorand node itself is written in Go, so the codebase can be imported, but needs
to be understood first.

READ THIS
For now, these tests assume there is an Algorand node running with connection parameters
and one funded account as specified below.
You can fulfill these assumptions by running sandbox: https://github.com/algorand/sandbox
I didn't want to pollute this codebase with loads of sandbox files, as eventually
we're moving to the import go-algorand approach anyway.
*/
package milk_e2e

type TestConfig struct {
	algodUrl      string
	algodToken    string
	senderPrivKey string
}

var testConfig = TestConfig{
	algodUrl:   "http://localhost:4001",
	algodToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	// address OGXUY4C5FWWMN3V7W5VE343LUW36MMIG6FZGUATW4RTVV65XMTOQCMRWFQ
	senderPrivKey: "08d800177ec604b2d384703dc52cccb5d77f284e6e35cb713a3cadc906a9f6d671af4c705d2dacc6eebfb76a4df36ba5b7e63106f1726a0276e4675afbb764dd",
}
