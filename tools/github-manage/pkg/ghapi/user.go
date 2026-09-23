package ghapi

import (
	"encoding/json"
	"fmt"
	"sync"
)

var username_mapping = map[string]string{}
var username_mapping_mu sync.Mutex

// ParseJSONtoUser converts JSON data to a slice of Issue structs.
func ParseJSONtoUser(jsonData []byte) (User, error) {
	var user User
	err := json.Unmarshal(jsonData, &user)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// GetUserName fetches a GitHub user's login and display name, caching results in-process.
func GetUserName(userLogin string) (User, error) {
	var user User

	username_mapping_mu.Lock()
	fullName, ok := username_mapping[userLogin]
	username_mapping_mu.Unlock()
	if ok {
		user.Login = userLogin
		user.Name = fullName
		return user, nil
	}

	command := fmt.Sprintf("gh api /users/%s", userLogin)

	results, err := RunCommandAndReturnOutput(command)
	if err != nil {
		return User{}, fmt.Errorf("fetch github user %q: %w", userLogin, err)
	}
	user, err = ParseJSONtoUser(results)
	if err != nil {
		return User{}, err
	}
	username_mapping_mu.Lock()
	username_mapping[user.Login] = user.Name
	username_mapping_mu.Unlock()
	return user, nil
}

func init() {
	// Ensure any package-level initialization here if needed
	username_mapping = make(map[string]string)
}
