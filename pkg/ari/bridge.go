package ari

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/url"
)

func (c *Client) BridgeAddChannel(brid, chid string) error {
	values := url.Values{}
	values.Set("channel", chid)

	res, err := c.Post(
		fmt.Sprintf("/ari/bridges/%s/addChannel", brid),
		"",
		&values,
		nil,
	)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	return nil
}

func (c *Client) BridgeCreate() (string, error) {
	brid := uuid.NewString()
	res, err := c.Post(
		fmt.Sprintf("/ari/bridges/%s", brid),
		"",
		nil,
		nil,
	)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	return brid, nil
}

func (c *Client) BridgeDelete(brid string) error {
	res, err := c.Do(
		"DELETE",
		fmt.Sprintf("/ari/bridges/%s", brid),
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
