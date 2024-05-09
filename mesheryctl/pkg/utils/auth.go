package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/layer5io/meshery/mesheryctl/internal/cli/root/config"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Provider struct {
	ProviderURL  string `json:"provider_url,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
}

// NewRequest creates *http.Request and handles adding authentication for Meshery itself
// Function returns a http response generated by the new request
func NewRequest(method string, url string, body io.Reader) (*http.Request, error) {
	// create new request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, ErrCreatingRequest(err)
	}

	// Grab token from the flag --token
	tokenPath := TokenFlag
	if tokenPath == "" { // token was not passed with the flag
		tokenPath, err = GetCurrentAuthToken()
		if err != nil {
			return nil, err
		}
		// set TokenFlag value equals tokenPath
		TokenFlag = tokenPath
	}
	// make sure if token-file exists
	exist, err := CheckFileExists(tokenPath)
	if err != nil || !exist {
		return nil, ErrAttachAuthToken(err)
	}

	log.Debug("token path is" + tokenPath)

	// add token to request
	err = AddAuthDetails(req, tokenPath)
	if err != nil {
		return nil, ErrAttachAuthToken(err)
	}

	return req, nil
}

// Function returns a new http response given a http request
// Function will test the response and return any errors associated with it
func MakeRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{}

	// check status code from request, checks for issues with auth token
	resp, err := client.Do(req)

	if err != nil && resp == nil {
		return nil, ErrFailRequest(err)
	}

	// If statuscode = 302, then we either have an expired or invalid token
	// We return the response and correct error message
	if resp.StatusCode == 302 {
		return nil, ErrInvalidToken()
	}

	// failsafe for not being authenticated
	if ContentTypeIsHTML(resp) {
		return nil, ErrUnauthenticated()
	}

	// failsafe for bad api call
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, ErrFailReqStatus(resp.StatusCode)
	}

	return resp, nil
}

// Function checks the location of token and returns appropriate location of the token
func GetTokenLocation(token config.Token) (string, error) {
	// Find home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get users home directory")
	}

	location := token.Location
	// if location contains /home/nectar in path we return exact location
	ok := strings.Contains(location, home)
	if ok {
		return location, nil
	}

	// else we add the home directory with the path
	return filepath.Join(MesheryFolder, location), nil
}

// GetCurrentAuthToken returns location of current context token
func GetCurrentAuthToken() (string, error) {
	// get config.yaml struct
	mctlCfg, err := config.GetMesheryCtl(viper.GetViper())
	if err != nil {
		log.Fatal(err)
	}
	// Get token of current-context
	token, err := mctlCfg.GetTokenForContext(mctlCfg.CurrentContext)
	if err != nil {
		// Attempt to create token if it doesn't already exists
		token.Location = AuthConfigFile

		// Write new entry in the config
		if err := config.AddTokenToConfig(token, DefaultConfigPath); err != nil {
			return "", err
		}
	}
	// grab actual token location with home directory
	TokenLocation, err := GetTokenLocation(token)
	if err != nil {
		return "", err
	}

	return TokenLocation, nil
}

// AddAuthDetails Adds authentication cookies to the request
func AddAuthDetails(req *http.Request, filepath string) error {
	file, err := os.ReadFile(filepath)
	if err != nil {
		err = errors.Wrap(err, "could not read token: ")
		return err
	}
	var tokenObj map[string]string
	if err := json.Unmarshal(file, &tokenObj); err != nil {
		err = errors.Wrap(err, "token file invalid: ")
		return err
	}
	req.AddCookie(&http.Cookie{
		Name:     tokenName,
		Value:    tokenObj[tokenName],
		HttpOnly: true,
	})
	req.AddCookie(&http.Cookie{
		Name:     providerName,
		Value:    tokenObj[providerName],
		HttpOnly: true,
	})
	return nil
}

// UpdateAuthDetails checks gets the token (old/refreshed) from meshery server and writes it back to the config file
func UpdateAuthDetails(filepath string) error {
	mctlCfg, err := config.GetMesheryCtl(viper.GetViper())
	if err != nil {
		return ErrLoadConfig(err)
	}

	// TODO: get this from the global config
	req, err := http.NewRequest("GET", mctlCfg.GetBaseMesheryURL()+"/api/user/token", bytes.NewBuffer([]byte("")))
	if err != nil {
		err = errors.Wrap(err, "error Creating the request: ")
		return err
	}
	if err := AddAuthDetails(req, filepath); err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	defer SafeClose(resp.Body)

	if err != nil {
		err = errors.Wrap(err, "error dispatching there request: ")
		return err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "error reading body: ")
		return err
	}

	if ContentTypeIsHTML(resp) {
		return errors.New("invalid body")
	}

	return os.WriteFile(filepath, data, os.ModePerm)
}

// ReadToken returns a map of the token passed in
func ReadToken(filepath string) (map[string]string, error) {
	file, err := os.ReadFile(filepath)
	if err != nil {
		err = errors.Wrap(err, "could not read token: ")
		return nil, ErrFileRead(err)
	}
	var tokenObj map[string]string
	if err := json.Unmarshal(file, &tokenObj); err != nil {
		err = errors.Wrap(err, "token file invalid: ")
		return nil, ErrUnmarshal(err)
	}
	return tokenObj, nil
}

// CreateTempAuthServer creates a temporary http server
//
// It implements a custom mux and has a catch all route, the function passed as the
// parameter is binded to the catch all route
func CreateTempAuthServer(fn func(http.ResponseWriter, *http.Request)) (*http.Server, int, error) {
	mux := http.NewServeMux()
	srv := &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp4", ":0")
	if err != nil {
		return nil, -1, err
	}

	mux.HandleFunc("/", fn)

	go func() {
		if err := srv.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				log.Println("error creating temporary server")
			}
		}
	}()

	return srv, listener.Addr().(*net.TCPAddr).Port, nil
}

// InitiateLogin initates the login process
func InitiateLogin(mctlCfg *config.MesheryCtlConfig, option string) ([]byte, error) {
	// Get the providers info
	providers, err := GetProviderInfo(mctlCfg)
	if err != nil {
		return nil, err
	}

	// Let the user select a provider
	var provider Provider
	if option != "" {
		// If option is given by user
		provider, err = chooseDirectProvider(providers, option)
	} else {
		// Trigger prompt
		provider = selectProviderPrompt(providers)
	}

	if err != nil {
		return nil, err
	}

	var token string

	log.Println("Initiating login...")

	// If the provider URL is empty then local provider
	if provider.ProviderURL == "" {
		token, err = initiateLocalProviderAuth(provider)
		if err != nil {
			return nil, err
		}
	} else {
		token, err = initiateRemoteProviderAuth(provider)
		if err != nil {
			return nil, err
		}
	}

	// Send request with the token to the meshery server
	data, err := getTokenObjFromMesheryServer(mctlCfg, provider.ProviderName, token)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// GetProviderInfo queries meshery API for the provider info
func GetProviderInfo(mctCfg *config.MesheryCtlConfig) (map[string]Provider, error) {
	res := map[string]Provider{}

	resp, err := http.Get(mctCfg.GetBaseMesheryURL() + "/api/providers")
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res, nil
}

// initiateLocalProviderAuth initiates login process for the local provider
func initiateLocalProviderAuth(_ Provider) (string, error) {
	return "", nil
}

// initiateRemoteProviderAuth intiates login process for the remote provider
func initiateRemoteProviderAuth(provider Provider) (string, error) {
	tokenChan := make(chan string, 1)

	// Create temporary server
	srv, port, err := CreateTempAuthServer(func(rw http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			fmt.Fprintf(rw, "token not found")
			return
		}

		fmt.Fprint(rw, "successfully logged in, you can close this window now")
		tokenChan <- token
	})
	if err != nil {
		return "", err
	}

	// Create provider URI
	uri, err := createProviderURI(provider, "http://localhost", port)
	if err != nil {
		return "", err
	}

	// Redirect user to the provider page
	if err := NavigateToBrowser(uri); err != nil {
		return "", err
	}

	// Pause until we get the response on the channel
	token := <-tokenChan

	// Shut down the server
	if err := srv.Shutdown(context.TODO()); err != nil {
		return token, err
	}

	return token, nil
}

func selectProviderPrompt(provs map[string]Provider) Provider {
	provArray := []Provider{}
	provNames := []string{}

	for _, prov := range provs {
		provArray = append(provArray, prov)
	}

	for _, prov := range provArray {
		provNames = append(provNames, prov.ProviderName)
	}

	prompt := promptui.Select{
		Label: "Select a Provider",
		Items: provNames,
	}

	for {
		i, _, err := prompt.Run()
		if err != nil {
			continue
		}

		return provArray[i]
	}
}

func chooseDirectProvider(provs map[string]Provider, option string) (Provider, error) {
	provArray := []Provider{}
	provNames := []string{}

	for _, prov := range provs {
		provArray = append(provArray, prov)
	}

	for _, prov := range provArray {
		provNames = append(provNames, prov.ProviderName)
	}

	for i := range provNames {
		if strings.EqualFold(provNames[i], option) {
			return provArray[i], nil
		}
	}
	return provArray[1], fmt.Errorf("the specified provider '%s' is not available. Please try giving correct provider name", option)
}

func createProviderURI(provider Provider, host string, port int) (string, error) {
	uri, err := url.Parse(provider.ProviderURL + "/login")
	if err != nil {
		return "", err
	}

	address := fmt.Sprintf("%s:%d", host, port)

	q := uri.Query()
	q.Add("source", base64.RawURLEncoding.EncodeToString([]byte(address)))
	q.Add("provider_version", "v0.3.14")

	uri.RawQuery = q.Encode()
	return uri.String(), nil
}

func getTokenObjFromMesheryServer(mctl *config.MesheryCtlConfig, provider, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, mctl.GetBaseMesheryURL()+"/api/token", nil)
	if err != nil {
		return nil, err
	}

	req.AddCookie(&http.Cookie{
		Name:     tokenName,
		Value:    token,
		HttpOnly: true,
	})
	req.AddCookie(&http.Cookie{
		Name:     "meshery-provider",
		Value:    provider,
		HttpOnly: true,
	})

	cli := &http.Client{}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func IsServerRunning(serverAddr string) error {
	serverAddr = strings.TrimPrefix(serverAddr, "http://")
	// Attempt to establish a connection to the server
	conn, err := net.DialTimeout("tcp", serverAddr, 2*time.Second)
	if err != nil {
		// Connection failed, server is not running
		return errors.WithMessage(err, "Meshery server is not reachable")
	}
	defer conn.Close()
	return nil
}
