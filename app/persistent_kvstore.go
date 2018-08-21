package kvstore

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/tendermint/tendermint/abci/example/code"
	"github.com/tendermint/tendermint/abci/types"
	cmn "github.com/tendermint/tendermint/libs/common"
	dbm "github.com/tendermint/tendermint/libs/db"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	ValidatorSetChangePrefix string = "val:"
	ValidatorAccountPrefix   string = "valacc:"
)

//-----------------------------------------

var _ types.Application = (*PersistentKVStoreApplication)(nil)

type PersistentKVStoreApplication struct {
	app *KVStoreApplication

	// validator set
	ValUpdates []types.Validator

	logger log.Logger
}

func NewPersistentKVStoreApplication(dbDir string) *PersistentKVStoreApplication {
	name := "kvstore"
	db, err := dbm.NewGoLevelDB(name, dbDir)
	if err != nil {
		panic(err)
	}

	state := loadState(db)

	return &PersistentKVStoreApplication{
		app:    &KVStoreApplication{state: state},
		logger: log.NewNopLogger(),
	}
}

func (app *PersistentKVStoreApplication) SetLogger(l log.Logger) {
	app.logger = l
}

func (app *PersistentKVStoreApplication) Info(req types.RequestInfo) types.ResponseInfo {
	res := app.app.Info(req)
	res.LastBlockHeight = app.app.state.Height
	res.LastBlockAppHash = app.app.state.AppHash
	return res
}

func (app *PersistentKVStoreApplication) SetOption(req types.RequestSetOption) types.ResponseSetOption {
	return app.app.SetOption(req)
}

// tx is either "val:pubkey/power" or "key=value" or just arbitrary bytes
func (app *PersistentKVStoreApplication) DeliverTx(tx []byte) types.ResponseDeliverTx {
	// if it starts with "val:", update the validator set
	// format is "val:pubkey/power"
	if isValidatorTx(tx) {
		// update validators in the merkle tree
		// and in app.ValUpdates
		return app.execValidatorTx(tx)
	} else if isValidatorAccountTx(tx) {
		return app.execValidatorAccountTx(tx)
	}

	// otherwise, update the key-value store
	return app.app.DeliverTx(tx)
}

func (app *PersistentKVStoreApplication) CheckTx(tx []byte) types.ResponseCheckTx {
	return app.app.CheckTx(tx)
}

// Commit will panic if InitChain was not called
func (app *PersistentKVStoreApplication) Commit() types.ResponseCommit {
	return app.app.Commit()
}

func (app *PersistentKVStoreApplication) Query(reqQuery types.RequestQuery) types.ResponseQuery {
	return app.app.Query(reqQuery)
}

// Save the validators in the merkle tree
func (app *PersistentKVStoreApplication) InitChain(req types.RequestInitChain) types.ResponseInitChain {
	for _, v := range req.Validators {
		r := app.updateValidator(v)
		if r.IsErr() {
			app.logger.Error("Error updating validators", "r", r)
		}
	}
	return types.ResponseInitChain{}
}

// Track the block hash and header information
func (app *PersistentKVStoreApplication) BeginBlock(req types.RequestBeginBlock) types.ResponseBeginBlock {
	// reset valset changes
	app.ValUpdates = make([]types.Validator, 0)
	return types.ResponseBeginBlock{}
}

// Update the validator set
func (app *PersistentKVStoreApplication) EndBlock(req types.RequestEndBlock) types.ResponseEndBlock {
	// update validators with account
	itr := app.app.state.db.Iterator(nil, nil)
	for ; itr.Valid(); itr.Next() {
		if isValidatorAccountTx(itr.Key()) {
			pubkey := itr.Key()[len(ValidatorAccountPrefix):]
			accountS := string(itr.Value())
			// fetch the balance
			balance, err := BalanceOf(accountS)
			if err != nil {
				continue
			}
			// calculate power by balance
			power := app.calculatePower(int(balance))

			//TODO: skip if power not changed
			// update validator
			app.updateValidator(types.Ed25519Validator(pubkey, int64(power)))
		}
	}
	return types.ResponseEndBlock{ValidatorUpdates: app.ValUpdates}
}

//---------------------------------------------
// update validators

func (app *PersistentKVStoreApplication) Validators() (validators []types.Validator) {
	itr := app.app.state.db.Iterator(nil, nil)
	for ; itr.Valid(); itr.Next() {
		if isValidatorTx(itr.Key()) {
			validator := new(types.Validator)
			err := types.ReadMessage(bytes.NewBuffer(itr.Value()), validator)
			if err != nil {
				panic(err)
			}
			validators = append(validators, *validator)
		}
	}
	return
}

func MakeValSetChangeTx(pubkey types.PubKey, power int64) []byte {
	return []byte(cmn.Fmt("val:%X/%d", pubkey.Data, power))
}

func isValidatorTx(tx []byte) bool {
	return strings.HasPrefix(string(tx), ValidatorSetChangePrefix)
}

func isValidatorAccountTx(tx []byte) bool {
	return strings.HasPrefix(string(tx), ValidatorAccountPrefix)
}

// format is "val:pubkey/power"
// pubkey is raw 32-byte ed25519 key
func (app *PersistentKVStoreApplication) execValidatorTx(tx []byte) types.ResponseDeliverTx {
	tx = tx[len(ValidatorSetChangePrefix):]

	//get the pubkey and power
	pubKeyAndPower := strings.Split(string(tx), "/")
	if len(pubKeyAndPower) != 2 {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Expected 'pubkey/power'. Got %v", pubKeyAndPower)}
	}
	pubkeyS, powerS := pubKeyAndPower[0], pubKeyAndPower[1]

	// decode the pubkey
	pubkey, err := hex.DecodeString(pubkeyS)
	if err != nil {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Pubkey (%s) is invalid hex", pubkeyS)}
	}

	// decode the power
	power, err := strconv.ParseInt(powerS, 10, 64)
	if err != nil {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Power (%s) is not an int", powerS)}
	}

	// update
	return app.updateValidator(types.Ed25519Validator(pubkey, int64(power)))
}

// format is "val:pubkey/account"
// pubkey is raw 32-byte ed25519 key
// account is an external address of token as stake
func (app *PersistentKVStoreApplication) execValidatorAccountTx(tx []byte) types.ResponseDeliverTx {
	tx = tx[len(ValidatorAccountPrefix):]

	//get the pubkey and account
	pubKeyAndAccount := strings.Split(string(tx), "/")
	if len(pubKeyAndAccount) != 2 {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Expected 'pubkey/account'. Got %v", pubKeyAndAccount)}
	}
	pubkeyS, accountS := pubKeyAndAccount[0], pubKeyAndAccount[1]

	// decode the pubkey
	pubkey, err := hex.DecodeString(pubkeyS)
	if err != nil {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Pubkey (%s) is invalid hex", pubkeyS)}
	}

	// fetch the balance
	balance, err := BalanceOf(accountS)
	fmt.Println(balance)
	if err != nil {
		return types.ResponseDeliverTx{
			Code: code.CodeTypeEncodingError,
			Log:  fmt.Sprintf("Cannot fetch balance from address (%s)", accountS)}
	}

	// update account
	app.updateAccount(pubkey, accountS)

	// calculate power by balance
	power := app.calculatePower(int(balance))

	// update validator
	return app.updateValidator(types.Ed25519Validator(pubkey, int64(power)))
}

func (app *PersistentKVStoreApplication) calculatePower(stake int) int {
	// TODO: stake to power
	return 10 + stake/100
}

func (app *PersistentKVStoreApplication) updateAccount(pubkey []byte, acc string) {
	key := []byte("valacc:" + string(pubkey))
	app.app.state.db.Set(key, []byte(acc))
}

func (app *PersistentKVStoreApplication) deleteAccount(pubkey []byte) {
	key := []byte("valacc:" + string(pubkey))
	app.app.state.db.Delete(key)
}

// add, update, or remove a validator
func (app *PersistentKVStoreApplication) updateValidator(v types.Validator) types.ResponseDeliverTx {
	key := []byte("val:" + string(v.PubKey.Data))
	if v.Power == 0 {
		// remove validator
		if !app.app.state.db.Has(key) {
			return types.ResponseDeliverTx{
				Code: code.CodeTypeUnauthorized,
				Log:  fmt.Sprintf("Cannot remove non-existent validator %X", key)}
		}
		app.app.state.db.Delete(key)
		// remove account
		app.deleteAccount(v.PubKey.Data)
	} else {
		// add or update validator
		value := bytes.NewBuffer(make([]byte, 0))
		if err := types.WriteMessage(&v, value); err != nil {
			return types.ResponseDeliverTx{
				Code: code.CodeTypeEncodingError,
				Log:  fmt.Sprintf("Error encoding validator: %v", err)}
		}
		app.app.state.db.Set(key, value.Bytes())
	}

	// we only update the changes array if we successfully updated the tree
	app.ValUpdates = append(app.ValUpdates, v)

	return types.ResponseDeliverTx{Code: code.CodeTypeOK}
}
