package suite

import (
	"context"
	"time"

	"github.com/fintrace-project/fintrace-tests/smoke/internal/apiclient"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/auth"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/db"
	"github.com/fintrace-project/fintrace-tests/smoke/internal/report"
)

// Target is what every application's suite runs against.
type Target struct {
	Client *apiclient.Client
	Token  auth.Token
	DB     *db.DB
	Report *report.Report
	Zone   *time.Location
}

type Func func(ctx context.Context, t Target)
