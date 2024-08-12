package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"git.defalsify.org/vise.git/cache"
	"git.defalsify.org/vise.git/engine"
	"git.defalsify.org/vise.git/persist"
	"git.defalsify.org/vise.git/resource"
	"git.defalsify.org/vise.git/state"
	testdataloader "github.com/peteole/testdata-loader"
)

const (
	createAccountURL = "https://custodial.sarafu.africa/api/account/create"
	trackStatusURL   = "https://custodial.sarafu.africa/api/track/"
)

const (
	USERFLAG_ACCOUNT_CREATED = iota + state.FLAG_USERSTART
	USERFLAG_ACCOUNT_PENDING
	USERFLAG_ACCOUNT_SUCCESS
)

type accountResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		CustodialId json.Number `json:"custodialId"`
		PublicKey   string      `json:"publicKey"`
		TrackingId  string      `json:"trackingId"`
	} `json:"result"`
}

type trackStatusResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		Transaction struct {
			CreatedAt     time.Time   `json:"createdAt"`
			Status        string      `json:"status"`
			TransferValue json.Number `json:"transferValue"`
			TxHash        string      `json:"txHash"`
			TxType        string      `json:"txType"`
		}
	} `json:"result"`
}

type accountResource struct {
	path  string
	state *state.State
}

var (
	baseDir     = testdataloader.GetBasePath()
	scriptDir   = path.Join(baseDir, "examples", "account")
	emptyResult = resource.Result{}
)

func (ar *accountResource) create_account(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	res := resource.Result{}
	fp := ar.path + "_data"
	f, err := os.OpenFile(fp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return res, err
	}
	f.Close()

	accountResp, err := createAccount()

	if err != nil {
		fmt.Println("Failed to create account:", err)
		return emptyResult, err
	}

	accountData := map[string]string{
		"TrackingId":  accountResp.Result.TrackingId,
		"PublicKey":   accountResp.Result.PublicKey,
		"CustodialId": accountResp.Result.CustodialId.String(),
		"Status":      "PENDING",
	}

	jsonData, err := json.Marshal(accountData)
	if err != nil {
		return res, err
	}

	err = os.WriteFile(fp, jsonData, 0644)
	if err != nil {
		return res, err
	}

	res.FlagSet = []uint32{USERFLAG_ACCOUNT_CREATED}
	return res, err
}

func (ar *accountResource) checkIdentifier(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	res := resource.Result{}
	fp := ar.path + "_data"

	
	jsonData, err := os.ReadFile(fp)
	if err != nil {
		return res, err
	}

	var accountData map[string]string
	err = json.Unmarshal(jsonData, &accountData)
	if err != nil {
		return res, err
	}

	res.Content = accountData["PublicKey"]

	return res, nil
}

func (ar *accountResource) check_account_status(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	res := resource.Result{}
	fp := ar.path + "_data"

	jsonData, err := os.ReadFile(fp)
	if err != nil {
		return res, err
	}

	var accountData map[string]string
	err = json.Unmarshal(jsonData, &accountData)
	if err != nil {
		return res, err
	}

	status, err := checkAccountStatus(accountData["TrackingId"])

	if err != nil {
		fmt.Println("Error checking account status:", err)
		return res, nil
	}

	accountData["Status"] = status

	if status == "SUCCESS" {
		res.FlagSet = []uint32{USERFLAG_ACCOUNT_SUCCESS}
		res.FlagReset = []uint32{USERFLAG_ACCOUNT_PENDING}
	} else {
		res.FlagSet = []uint32{USERFLAG_ACCOUNT_PENDING}
		res.FlagReset = []uint32{USERFLAG_ACCOUNT_SUCCESS}
	}

	updatedJsonData, err := json.Marshal(accountData)
	if err != nil {
		return res, err
	}

	err = os.WriteFile(fp, updatedJsonData, 0644)
	if err != nil {
		return res, err
	}

	return res, nil
}

func createAccount() (*accountResponse, error) {
	resp, err := http.Post(createAccountURL, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var accountResp accountResponse
	err = json.Unmarshal(body, &accountResp)
	if err != nil {
		return nil, err
	}

	return &accountResp, nil
}

func checkAccountStatus(trackingId string) (string, error) {
	resp, err := http.Get(trackStatusURL + trackingId)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var trackResp trackStatusResponse
	err = json.Unmarshal(body, &trackResp)
	if err != nil {
		return "", err
	}

	status := trackResp.Result.Transaction.Status

	return status, nil
}

func main() {
	var root string
	var size uint
	var sessionId string
	flag.UintVar(&size, "s", 0, "max size of output")
	flag.StringVar(&root, "root", "root", "entry point symbol")
	flag.StringVar(&sessionId, "session-id", "default", "session id")
	flag.Parse()
	fmt.Fprintf(os.Stderr, "starting session at symbol '%s' using resource dir: %s\n", root, scriptDir)

	ctx := context.Background()
	st := state.NewState(3)
	rfs := resource.NewFsResource(scriptDir)
	ca := cache.NewCache()
	cfg := engine.Config{
		Root:      "root",
		SessionId: sessionId,
	}

	dp := path.Join(scriptDir, ".state")
	err := os.MkdirAll(dp, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "state dir create exited with error: %v\n", err)
		os.Exit(1)
	}
	pr := persist.NewFsPersister(dp)
	en, err := engine.NewPersistedEngine(ctx, cfg, pr, rfs)
	if err != nil {
		pr = pr.WithContent(&st, ca)
		err = pr.Save(cfg.SessionId)
		en, err = engine.NewPersistedEngine(ctx, cfg, pr, rfs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engine create exited with error: %v\n", err)
			os.Exit(1)
		}
	}

	fp := path.Join(dp, sessionId)
	rs := accountResource{
		path:  fp,
		state: &st,
	}
	rfs.AddLocalFunc("create_account", rs.create_account)
	rfs.AddLocalFunc("check_identifier", rs.checkIdentifier)
	rfs.AddLocalFunc("check_account_status", rs.check_account_status)

	cont, err := en.Init(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine init exited with error: %v\n", err)
		os.Exit(1)
	}
	if !cont {
		_, err = en.WriteResult(ctx, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dead init write error: %v\n", err)
			os.Exit(1)
		}
		err = en.Finish()
		if err != nil {
			fmt.Fprintf(os.Stderr, "engine finish error: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write([]byte{0x0a})
		os.Exit(0)
	}
	err = engine.Loop(ctx, en, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loop exited with error: %v\n", err)
		os.Exit(1)
	}
}
