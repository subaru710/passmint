# passmint
a blockchain passport based on tendermint

## Install
```
# install via go get
go get github.com/subaru710/passmint

# install vendors
cd $GOPATH/src/github.com/subaru710/passmint
glide install

# install passmint
go install github.com/subaru710/passmint/cmd/...

```

## Walktrough

```
# start tendermint
tendermint init
tendermint node --consensus.create_empty_blocks=false

# start passmint in another terminal
passmint  -persist ~/.tendermint/data

## Set validator's external account
# "valacc:pubkey/account"
http://localhost:26657/broadcast_tx_commit?tx="valacc:${pubkey}/${account}"