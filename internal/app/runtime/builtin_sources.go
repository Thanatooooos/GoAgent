package runtime

import (
	"fmt"
	"strings"
	"time"
)

// Date provides the current calendar date as a stable system-context fact for
// every model turn. It deliberately renders at request time rather than at
// bootstrap, so a long-running server never sends a stale date.
func Date() ContextSource {
	return ContextSource{
		Key: "core/date",
		Render: func(SourceContext) string {
			return currentDateContextPrefix + time.Now().Format("Mon Jan 2 2006")
		},
	}
}

// LocalTime renders the user's own wall clock so relative requests such as
// "tomorrow at nine" resolve against the clock the user reads. It renders
// nothing without a usable zone: the scheduled-task tools deny every call in
// that case, and a source must never show a time they would refuse.
func LocalTime() ContextSource {
	return ContextSource{
		Key: "core/local-time",
		Render: func(source SourceContext) string {
			zone := strings.TrimSpace(source.Request.Timezone)
			if zone == "" {
				return ""
			}
			location, err := time.LoadLocation(zone)
			if err != nil {
				return ""
			}
			return fmt.Sprintf(localTimeContextTemplate,
				time.Now().In(location).Format("Mon Jan 2 2006 15:04"), zone)
		},
	}
}
