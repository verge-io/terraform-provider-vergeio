package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"terraform-provider-vergeio/internal/provider/vergeio"
)

type RecipeApi struct {
	Client *vergeio.Client
}

func NewRecipeApi(client *vergeio.Client) *RecipeApi {
	return &RecipeApi{Client: client}
}

type RecipeInstanceRequest struct {
	Name    string            `json:"name"`
	Recipe  string            `json:"recipe"`
	Answers map[string]string `json:"answers,omitempty"`
}

type RecipeInstanceResponse struct {
	Response struct {
		VM string `json:"vm"`
	} `json:"response"`
}

func (a *RecipeApi) CreateRecipeInstance(ctx context.Context, name string, recipeKey string, answers map[string]string) (string, error) {
	reqData := RecipeInstanceRequest{
		Name:    name,
		Recipe:  recipeKey,
		Answers: answers,
	}

	payload, err := json.Marshal(reqData)
	if err != nil {
		return "", err
	}

	resp, err := a.Client.Post("api/v4/vm_recipe_instances", bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}

	var recipeResp RecipeInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&recipeResp); err != nil {
		return "", err
	}

	return recipeResp.Response.VM, nil
}

func (a *RecipeApi) PowerOnVM(ctx context.Context, vmID string) error {
	_, err := a.Client.Put(fmt.Sprintf("api/v4/vms/%s", vmID), bytes.NewBufferString(`{"powerstate": true}`))
	return err
}

func (a *RecipeApi) DeleteVM(ctx context.Context, vmID string) error {
	// First power off
	_, _ = a.Client.Post(fmt.Sprintf("api/v4/vms/%s?action=poweroff", vmID), nil)

	// Then delete
	_, err := a.Client.Delete(fmt.Sprintf("api/v4/vms/%s", vmID))
	return err
}

func (a *RecipeApi) GetVM(ctx context.Context, vmID string) (map[string]interface{}, error) {
	resp, err := a.Client.Get(fmt.Sprintf("api/v4/vms/%s", vmID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
