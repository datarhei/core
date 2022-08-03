package client

import (
	"encoding/json"

	"github.com/datarhei/core/v16/http/api"
)

func (r *restclient) Skills() (api.Skills, error) {
	var skills api.Skills

	data, err := r.call("GET", "/skills", "", nil)
	if err != nil {
		return skills, err
	}

	err = json.Unmarshal(data, &skills)

	return skills, err
}

func (r *restclient) SkillsReload() error {
	_, err := r.call("GET", "/skills/reload", "", nil)

	return err
}
