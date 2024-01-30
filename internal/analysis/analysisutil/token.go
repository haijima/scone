package analysisutil

import (
	"fmt"
	"go/token"
	"path/filepath"
)

func FLC(position token.Position) string {
	return fmt.Sprintf("%s:%d:%d", filepath.Base(position.Filename), position.Line, position.Column)
}
