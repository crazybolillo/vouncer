package ari

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/url"
)

func (c *Client) ChannelSetVar(chid, name, value string) error {
	values := url.Values{}
	values.Set("variable", name)
	values.Set("value", value)

	res, err := c.Post(fmt.Sprintf("/ari/channels/%s/variable", chid),
		"",
		&values,
		nil,
	)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return nil
}

// ChannelDial creates a new channel and calls the specified endpoint. It returns the newly created channel ID.
func (c *Client) ChannelDial(endpoint, app string, params url.Values, chanVars map[string]string) (string, error) {
	chid := uuid.NewString()
	payload, err := json.Marshal(map[string]any{
		"variables": chanVars,
	})
	if err != nil {
		return "", err
	}

	params.Set("app", app)
	params.Set("endpoint", "PJSIP/"+endpoint)

	res, err := c.Post(
		fmt.Sprintf("/ari/channels/%s", chid),
		"application/json",
		&params,
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return chid, nil
}

func (c *Client) ChannelAnswer(chid string) error {
	res, err := c.Post(
		fmt.Sprintf("/ari/channels/%s/answer", chid),
		"",
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return nil
}

func (c *Client) ChannelDelete(chid string) error {
	res, err := c.Do(
		"DELETE",
		fmt.Sprintf("/ari/channels/%s", chid),
		"",
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	switch res.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
}

func (c *Client) ChannelRing(chid string, state bool) error {
	method := "POST"
	if !state {
		method = "DELETE"
	}

	res, err := c.Do(
		method,
		fmt.Sprintf("/ari/channels/%s/ring", chid),
		"",
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return nil
}

func (c *Client) ChannelPlay(chid string, media string) error {
	params := url.Values{}
	params.Set("media", media)

	res, err := c.Post(
		fmt.Sprintf("/ari/channels/%s/play", chid),
		"",
		&params,
		nil,
	)

	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return nil
}
