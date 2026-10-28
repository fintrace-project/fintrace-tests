package suite

import (
	"context"
	"time"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/auth"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/db"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/report"
)

// Target is what every application's suite runs against. Client calls the application under test;
// Core calls fintrace-core, which a suite for another application needs for setup and read-back.
type Target struct {
	Client *apiclient.Client
	Core   *apiclient.Client
	Token  auth.Token
	DB     *db.DB
	Report *report.Report
	Zone   *time.Location
	Dumps  string
	Issuer string
}

type Func func(ctx context.Context, t Target)
