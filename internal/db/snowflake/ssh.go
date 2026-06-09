package snowflake

import (
	"errors"

	"github.com/conray/dataseai/internal/db"
)

func (Snowflake) RegisterSSHDialer(db.SSHConfig, db.DSNInput) (string, func(), error) {
	return "", nil, errors.New("snowflake: SSH tunneling not supported; connect directly via account identifier")
}
