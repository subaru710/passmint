package kvstore

import (
	"fmt"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
	"strings"
)

const CONTRACT_ADDRESS = "0x63f584fa56e60e4d0fe8802b27c7e6e3b33e007f"
const FUNCTION_SIGNATURE = "0x70a08231000000000000000000000000"

type BalanceRequest struct {
	To   string `json:"to"`
	Data string `json:"data"`
}

type BalanceResponse struct {
	result string
}

func genRequestData(account string) (string, error) {
	if !strings.HasPrefix(account, "0x") {
		err := fmt.Errorf("%s", "invalid account")
		fmt.Println(err.Error())
		return "", err
	}
	return FUNCTION_SIGNATURE + account[2:], nil
}

func BalanceOf(account string) (int64, error) {
	client, err := rpc.Dial("http://node3.web3api.com")
	if err != nil {
		fmt.Println("rpc.Dial err", err)
		return 0, err
	}

	data, err := genRequestData(account)
	if err != nil {
		fmt.Println("genRequestData err", err)
		return 0, err
	}
	var result string
	err = client.Call(&result, "eth_call", &BalanceRequest{CONTRACT_ADDRESS, data}, "latest")
	if err != nil {
		fmt.Println("client.Call err", err)
		return 0, err
	}

	fmt.Printf("account: %s\nbalance: %s\n", account, result)
	wei := big.NewInt(0)
	wei, succ := wei.SetString(result, 0)
	if !succ {
		fmt.Println("parse big.Int err")
		return 0, err
	}
	fmt.Println(wei)
	t := big.NewInt(0)
	return t.Div(wei, big.NewInt(1000000000000000000)).Int64(), nil
}
