package main

import (
	"flag"
	"os"

	mapp "github.com/subaru710/passmint/app"
	"github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/abci/types"
	cmn "github.com/tendermint/tendermint/libs/common"
	"github.com/tendermint/tendermint/libs/log"
)

func main() {

	// mapp.BalanceOf("0x8546a5a4b3bbe86bf57fc9f5e497c770ae5d0233")
	flagAddress := flag.String("addr", "tcp://0.0.0.0:26658", "address of application socket")
	flagAbci := flag.String("abci", "socket", "either socket or grpc")
	flagPersist := flag.String("persist", "", "directory to use for a database")
	// flagAccount := flag.String("account", "0x8546a5a4b3bbe86bf57fc9f5e497c770ae5d0233", "account on eth")
	flag.Parse()

	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	// Create the application - in memory or persisted to disk
	var app types.Application
	if *flagPersist == "" {
		app = mapp.NewKVStoreApplication()
	} else {
		app = mapp.NewPersistentKVStoreApplication(*flagPersist)
		app.(*mapp.PersistentKVStoreApplication).SetLogger(logger.With("module", "kvstore"))
	}

	// Start the listener
	srv, err := server.NewServer(*flagAddress, *flagAbci, app)
	if err != nil {
		os.Exit(1)
	}
	srv.SetLogger(logger.With("module", "abci-server"))
	if err := srv.Start(); err != nil {
		os.Exit(1)
	}

	// Wait forever
	cmn.TrapSignal(func() {
		// Cleanup
		srv.Stop()
	})

}
