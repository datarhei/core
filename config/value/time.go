package value

import (
	"time"

	"github.com/lestrrat-go/strftime"
)

// time

type Time time.Time

func NewTime(p *time.Time, val time.Time) *Time {
	*p = val

	return (*Time)(p)
}

func (u *Time) Set(val string) error {
	v, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return err
	}
	*u = Time(v)
	return nil
}

func (u *Time) String() string {
	v := time.Time(*u)
	return v.Format(time.RFC3339)
}

func (u *Time) Validate() error {
	return nil
}

func (u *Time) IsEmpty() bool {
	v := time.Time(*u)
	return v.IsZero()
}

// strftime

type Strftime string

func NewStrftime(p *string, val string) *Strftime {
	*p = val

	return (*Strftime)(p)
}

func (s *Strftime) Set(val string) error {
	*s = Strftime(val)
	return nil
}

func (s *Strftime) String() string {
	return string(*s)
}

func (s *Strftime) Validate() error {
	_, err := strftime.New(string(*s))
	return err
}

func (s *Strftime) IsEmpty() bool {
	return len(string(*s)) == 0
}
