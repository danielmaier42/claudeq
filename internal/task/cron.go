package task

import (
	"fmt"
	"strings"
	"time"
)

// cronExample is the schedule used in error messages as a template to copy.
const cronExample = `"0 20 * * *" = every day at 20:00`

// cronFieldNames names the five crontab fields in order, with the values each
// one accepts. A rejected expression is reported per field, so whoever typed it
// learns which of the five is wrong instead of only that the line failed.
var cronFieldNames = [5]struct{ name, accepts string }{
	{"minute", "0-59"},
	{"hour", "0-23"},
	{"day-of-month", "1-31"},
	{"month", "1-12 or JAN-DEC"},
	{"weekday", "0-6 or SUN-SAT"},
}

// CheckCron reports whether expr is a crontab schedule claudeq can run. The
// error is written for the person who typed the expression: it names the field
// at fault and the values that field accepts, because the parser alone only
// says that the whole line is unparseable.
func CheckCron(expr string) error {
	spec := strings.TrimSpace(expr)
	if spec == "" {
		return fmt.Errorf("missing cron schedule (e.g. %s)", cronExample)
	}
	if strings.HasPrefix(spec, "@") {
		return fmt.Errorf("invalid cron %q: shorthands like @daily are not supported, use five fields (e.g. %s)", spec, cronExample)
	}
	fields := strings.Fields(spec)
	// A leading TZ=/CRON_TZ= token names the zone the schedule is read in. The
	// parser accepts it ahead of the five fields, so it must not be counted as
	// one of them — otherwise a working timezone-explicit schedule is rejected.
	if strings.HasPrefix(fields[0], "TZ=") || strings.HasPrefix(fields[0], "CRON_TZ=") {
		fields = fields[1:]
	}
	if len(fields) != 5 {
		return fmt.Errorf("invalid cron %q: needs exactly 5 fields (minute hour day-of-month month weekday), found %d",
			spec, len(fields))
	}
	if _, err := CronParser.Parse(spec); err != nil {
		// Pin the failure to one field by parsing that field alone with the other
		// four wildcarded; whichever isolated field still fails is the culprit.
		for i, f := range fields {
			only := []string{"*", "*", "*", "*", "*"}
			only[i] = f
			if _, ferr := CronParser.Parse(strings.Join(only, " ")); ferr != nil {
				fld := cronFieldNames[i]
				return fmt.Errorf("invalid cron %q: the %s field %q is not valid (%s accepts %s): %s",
					spec, fld.name, f, fld.name, fld.accepts, cronDetail(ferr, f))
			}
		}
		return fmt.Errorf("invalid cron %q: %w", spec, err)
	}
	return nil
}

// cronDetail is the parser's own explanation, minus the trailing repetition of
// the offending token the library appends ("above maximum (23): 99").
func cronDetail(err error, field string) string {
	return strings.TrimSuffix(err.Error(), ": "+field)
}

// CronNext returns the next n occurrences of expr after from. It fails for an
// expression CheckCron rejects.
func CronNext(expr string, from time.Time, n int) ([]time.Time, error) {
	if err := CheckCron(expr); err != nil {
		return nil, err
	}
	sched, err := CronParser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return nil, fmt.Errorf("invalid cron %q: %w", expr, err)
	}
	out := make([]time.Time, 0, n)
	at := from
	for i := 0; i < n; i++ {
		next := sched.Next(at)
		if next.IsZero() {
			break // no further occurrence exists (e.g. "0 0 30 2 *")
		}
		out = append(out, next)
		at = next
	}
	return out, nil
}
