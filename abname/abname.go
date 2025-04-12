package abname

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	_ "embed"
)

type ID int64

const MaxSurnameLength = 10
const ForenameBits = 63 - 5*MaxSurnameLength
const MaxForenameID = 1<<ForenameBits - 1

var ErrNotInitialized = errors.New("abname.ErrNotInitialized (missed calling Init?)")
var ErrNameTooLong = errors.New("abname.ErrNameTooLong")
var ErrInvalidChars = errors.New("abname.ErrInvalidChars")
var ErrBadForenameID = errors.New("abname.ErrBadForenameID")
var ErrUnknownForename = errors.New("abname.ErrUnknownForename")

// New converts name to ID.
// Returns 0 if name is too long or the suffix is not supported.
func New(name string) (ID, error) {
	var id ID
	sur, fore, _ := strings.Cut(name, "-")
	if sur == "" || len(sur) > MaxSurnameLength {
		return 0, ErrNameTooLong
	}
	for i := len(sur) - 1; i >= 0; i-- {
		if sur[i] < 'a' || 'z' < sur[i] {
			return 0, ErrInvalidChars
		}
		id = id<<5 + ID(sur[i]-'a'+1)
	}
	id <<= ForenameBits
	if fore == "" {
		return id, nil
	}

	if forename2id == nil {
		return 0, ErrNotInitialized
	}
	if '0' <= fore[0] && fore[0] <= '9' {
		num, err := strconv.Atoi(fore)
		if err != nil || num <= 0 || MaxForenameID < num {
			return 0, ErrBadForenameID
		}
		return id | ID(num), nil
	}

	forecode := ID(0)
	for i := len(fore) - 1; i >= 0; i-- {
		if fore[i] < 'a' || 'z' < fore[i] {
			return 0, ErrInvalidChars
		}
		forecode = forecode<<5 + ID(fore[i]-'a'+1)
	}
	foreid, ok := forename2id[forecode]
	if !ok {
		return 0, ErrUnknownForename
	}
	return id | foreid, nil
}

func (id ID) String() string {
	bs := make([]byte, 0, 24)
	for surid := id >> ForenameBits; surid > 0; surid >>= 5 {
		if surid&31 > 26 {
			return ""
		}
		bs = append(bs, byte('a'+surid&31-1))
	}

	foreid := id & MaxForenameID
	if foreid == 0 {
		return string(bs)
	}
	if id2forename == nil {
		panic("abnames.NotInitialized (missed calling Init?)")
	}
	forename, ok := id2forename[foreid]
	if !ok {
		return fmt.Sprintf("%s-%d", bs, foreid)
	}
	bs = append(bs, '-')
	for ; forename > 0; forename >>= 5 {
		if forename&31 > 26 {
			return ""
		}
		bs = append(bs, byte('a'+forename&31-1))
	}
	return string(bs)
}

// Avoid strings for memory efficiency reasons.
// Use the same 5-bit encoding for the forename strings too.
var forename2id map[ID]ID
var id2forename map[ID]ID

//go:embed forenames.txt
var forenames string

// Init initializes the abname module.
// Must be called before using any of the functions in this module.
func Init() error {
	lines := strings.Split(strings.TrimSpace(forenames), "\n")
	forename2id = make(map[ID]ID, len(lines))
	id2forename = make(map[ID]ID, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) != 2 {
			return fmt.Errorf("abname.InvalidTokenCount tokens=%d line=%q", len(fs), line)
		}
		forenameID, err := strconv.Atoi(fs[0])
		if err != nil {
			return fmt.Errorf("abname.ParseForenameIDNumber line=%q: %v", line, err)
		}
		if forenameID > MaxForenameID {
			return fmt.Errorf("abname.ForeameIDTooLarge limit=%d line=%q", MaxForenameID, forenameID)
		}
		if len(fs[1]) > MaxSurnameLength {
			return fmt.Errorf("abname.ForenameTooLong name=%s limit=%d", fs[1], MaxSurnameLength)
		}
		nameID, err := New(fs[1])
		if err != nil {
			return fmt.Errorf("abname.ParseForename line=%q: %v", line, err)
		}
		nameID >>= ForenameBits
		forename2id[nameID] = ID(forenameID)
		id2forename[ID(forenameID)] = nameID
	}
	return nil
}
