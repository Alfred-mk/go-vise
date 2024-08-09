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
	"git.defalsify.org/vise.git/resource"
	"git.defalsify.org/vise.git/state"
	testdataloader "github.com/peteole/testdata-loader"
)

const (
	USERFLAG_HASACCEPTED    = state.FLAG_USERSTART
	USERFLAG_HASSESSION     = state.FLAG_USERSTART
	USERFLAG_HASACCOUNT     = state.FLAG_USERSTART
	USERFLAG_ACCOUNTSUCCESS = state.FLAG_USERSTART
	createAccountURL        = "https://custodial.sarafu.africa/api/account/create"
	trackStatusURL          = "https://custodial.sarafu.africa/api/track/"
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

type UserState struct {
	CurrentState  int
	PublicKey     string
	CustodialId   string
	TrackingId    string
	AccountStatus string
}

var (
	baseDir     = testdataloader.GetBasePath()
	scriptDir   = path.Join(baseDir, "examples", "account")
	emptyResult = resource.Result{}
)

type accountResource struct {
	*resource.FsResource
	st *state.State
}

func saveUserState(sessionId string, state *UserState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	fp := path.Join(scriptDir, sessionId+"_userstate.json")
	err = os.WriteFile(fp, data, 0600)
	if err != nil {
		return err
	}
	return nil
}

func loadUserState(sessionId string) (*UserState, error) {
	fp := path.Join(scriptDir, sessionId+"_userstate.json")
	data, err := os.ReadFile(fp)
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

func updateState(sessionId string, updates map[string]interface{}) error {
	userState, err := loadUserState(sessionId)
	if err != nil {
		return err
	}

	for key, value := range updates {
		switch key {
		case "CurrentState":
			if v, ok := value.(int); ok {
				userState.CurrentState = v
			}
		case "PublicKey":
			if v, ok := value.(string); ok {
				userState.PublicKey = v
			}
		case "CustodialId":
			if v, ok := value.(string); ok {
				userState.CustodialId = v
			}
		case "TrackingId":
			if v, ok := value.(string); ok {
				userState.TrackingId = v
			}
		case "AccountStatus":
			if v, ok := value.(string); ok {
				userState.AccountStatus = v
			}
		default:
			return fmt.Errorf("unknown field: %s", key)
		}
	}

	return saveUserState(sessionId, userState)
}

func (ir *accountResource) check_session(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	var err error
	state := &UserState{}

	sessionId := ctx.Value("SessionId").(string)

	sessionFile, err := sessionExists(sessionId)
	if err != nil {
		return emptyResult, err
	}

	state.CurrentState = StateNone

	if sessionFile {
		ir.st.SetFlag(USERFLAG_HASSESSION)
	} else if !sessionFile {
		ir.st.ResetFlag(USERFLAG_HASSESSION)

		path.Join(scriptDir, sessionId+"_userstate.json")

		saveUserState(sessionId, state)

		return emptyResult, err
	}

	return resource.Result{
		Content: "",
	}, err
}

func sessionExists(sessionId string) (bool, error) {
	filePath := path.Join(scriptDir, sessionId+"_userstate.json")
	if _, err := os.Stat(filePath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func (ir *accountResource) accept_account(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	var err error
	sessionId := ctx.Value("SessionId").(string)
	updates := map[string]interface{}{
		"CurrentState": StateAccountAccepted,
	}

	updateState(sessionId, updates)

	return emptyResult, err
}

func (ir *accountResource) accept_terms(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	var err error
	sessionId := ctx.Value("SessionId").(string)
	state, err := loadUserState(sessionId)

	if err != nil {
		return emptyResult, err
	}

	if state.TrackingId == "" {
		accountResp, err := createAccount()

		if err != nil {
			fmt.Println("Failed to create account:", err)
			return emptyResult, err
		}

		updates := map[string]interface{}{
			"CurrentState": StateAccountCreationPending,
			"PublicKey":    accountResp.Result.PublicKey,
			"TrackingId":   accountResp.Result.TrackingId,
			"CustodialId":  accountResp.Result.CustodialId.String(),
		}

		updateState(sessionId, updates)
	}

	return emptyResult, err
}

func (ir *accountResource) checkIdentifier(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	sessionId := ctx.Value("SessionId").(string)

	state, err := loadUserState(sessionId)
	if err != nil {
		return emptyResult, err
	}

	r := resource.Result{
		Content: fmt.Sprintf(state.PublicKey),
	}
	return r, nil
}

func (ir *accountResource) check_account_creation(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	sessionId := ctx.Value("SessionId").(string)
	state, err := loadUserState(sessionId)

	if err != nil {
		return emptyResult, err
	}

	if state.TrackingId == "" {
		ir.st.ResetFlag(USERFLAG_HASACCOUNT)
		return emptyResult, err
	}

	ir.st.SetFlag(USERFLAG_HASACCOUNT)

	return resource.Result{
		Content: "",
	}, err
}

func (ir *accountResource) check_account_status(ctx context.Context, sym string, input []byte) (resource.Result, error) {
	sessionId := ctx.Value("SessionId").(string)
	state, err := loadUserState(sessionId)

	if err != nil {
		ir.st.ResetFlag(USERFLAG_ACCOUNTSUCCESS)
		return emptyResult, err
	}

	status, err := checkAccountStatus(state.TrackingId)
	if err != nil {
		ir.st.ResetFlag(USERFLAG_ACCOUNTSUCCESS)
		fmt.Println("Error checking account status:", err)
		return emptyResult, err
	}

	if status == "SUCCESS" {
		ir.st.SetFlag(USERFLAG_ACCOUNTSUCCESS)

		updates := map[string]interface{}{
			"AccountStatus": status,
			"CurrentState":  StateAccountCreated,
		}

		updateState(sessionId, updates)
	} else {
		ir.st.ResetFlag(USERFLAG_ACCOUNTSUCCESS)
		updates := map[string]interface{}{
			"AccountStatus": status,
			"CurrentState":  StateAccountCreationPending,
		}

		updateState(sessionId, updates)
		return emptyResult, err
	}

	return resource.Result{
		Content: "",
	}, err
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
	rs := accountResource{rsf, &st}
	rs.AddLocalFunc("check_session", rs.check_session)
	rs.AddLocalFunc("accept_account", rs.accept_account)
	rs.AddLocalFunc("accept_terms", rs.accept_terms)
	rs.AddLocalFunc("check_identifier", rs.checkIdentifier)
	rs.AddLocalFunc("check_account_creation", rs.check_account_creation)
	rs.AddLocalFunc("check_account_status", rs.check_account_status)
	ca := cache.NewCache()
	cfg := engine.Config{
		Root:       "root",
		SessionId:  sessionId,
		OutputSize: uint32(size),
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "SessionId", sessionId)
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
