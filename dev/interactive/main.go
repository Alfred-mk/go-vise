package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"git.defalsify.org/vise.git/cache"
	"git.defalsify.org/vise.git/engine"
	"git.defalsify.org/vise.git/resource"
	"git.defalsify.org/vise.git/state"
	testdataloader "github.com/peteole/testdata-loader"
)

const (
	USERFLAG_HASACCEPTED = state.FLAG_USERSTART
)

const (
	StateNone = iota
	StateAccountAccepted
	StateTermsOffered
	StateTermsAccepted
	StateAccountCreationPending
	StateAccountCreated
)

type accountResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		CustodialId json.Number `json:"custodialId"`
		PublicKey   string      `json:"publicKey"`
		TrackingId  string      `json:"trackingId"`
	} `json:"result"`
}

type UserState struct {
	CurrentState int
	PublicKey    string
	CustodialId  string
	TrackingId   string
}

var (
	baseDir     = testdataloader.GetBasePath()
	scriptDir   = path.Join(baseDir, "dev", "interactive")
	emptyResult = resource.Result{}
)

type interactiveResource struct {
	*resource.FsResource
	st *state.State
}

func saveUserState(state *UserState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	fp := path.Join(scriptDir, "userstate.json")
	err = ioutil.WriteFile(fp, data, 0600)
	if err != nil {
		return err
	}
	return nil
}

func loadUserState() (*UserState, error) {
	fp := path.Join(scriptDir, "userstate.json")
	data, err := ioutil.ReadFile(fp)
	if err != nil {
		return nil, err
	}

	var state UserState
	err = json.Unmarshal(data, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

func (ir *interactiveResource) accept_account(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	var err error
	state := &UserState{CurrentState: StateNone}
	if string(input) == "accept" {
		state.CurrentState = StateAccountAccepted
		ir.st.SetFlag(USERFLAG_HASACCEPTED)

		saveUserState(state)
	}
	return resource.Result{
		Content: "",
	}, err
}

func (ir *interactiveResource) accept_terms(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	var err error
	state := &UserState{CurrentState: StateNone}
	ir.st.ResetFlag(USERFLAG_HASACCEPTED)
	if string(input) == "accept" {
		state.CurrentState = StateTermsAccepted
		ir.st.SetFlag(USERFLAG_HASACCEPTED)

		fmt.Println("Account creation is in progress, please wait...")

		accountResp, err := createAccount()
		if err != nil {
			fmt.Println("Failed to create account:", err)
			return emptyResult, err
		}

		state.PublicKey = accountResp.Result.PublicKey
		state.TrackingId = accountResp.Result.TrackingId
		state.CustodialId = accountResp.Result.CustodialId.String()

		state.CurrentState = StateAccountCreated

		saveUserState(state)

		return emptyResult, err
	}

	return emptyResult, err
}

func (ir *interactiveResource) checkIdentifier(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	state, err := loadUserState()
	if err != nil {
		return emptyResult, err
	}

	r := resource.Result{
		Content: fmt.Sprintf(state.PublicKey),
	}
	return r, nil
}

func createAccount() (*accountResponse, error) {
	resp, err := http.Post("https://custodial.sarafu.africa/api/account/create", "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
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

func main() {
	var dir string
	var root string
	var size uint
	var sessionId string
	flag.UintVar(&size, "s", 0, "max size of output")
	flag.StringVar(&root, "root", "root", "entry point symbol")
	flag.StringVar(&sessionId, "session-id", "default", "session id")
	flag.Parse()
	fmt.Fprintf(os.Stderr, "starting session at symbol '%s' using resource dir: %s\n", root, dir)

	st := state.NewState(1)
	rsf := resource.NewFsResource(scriptDir)
	rs := interactiveResource{rsf, &st}
	rs.AddLocalFunc("accept_account", rs.accept_account)
	rs.AddLocalFunc("accept_terms", rs.accept_terms)
	rs.AddLocalFunc("check_identifier", rs.checkIdentifier)
	ca := cache.NewCache()
	cfg := engine.Config{
		Root:       "root",
		SessionId:  sessionId,
		OutputSize: uint32(size),
	}
	ctx := context.Background()
	en := engine.NewEngine(ctx, cfg, &st, rs, ca)
	var err error
	_, err = en.Init(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine init fail: %v\n", err)
		os.Exit(1)
	}

	err = engine.Loop(ctx, &en, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loop exited with error: %v\n", err)
		os.Exit(1)
	}
}
