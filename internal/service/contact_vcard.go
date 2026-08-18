package service

import (
	"context"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

const (
	vcardCRLF = "\r\n"
	// vcardFoldWidth is RFC 2426's 75-octet line limit. Continuation lines
	// begin with a single space.
	vcardFoldWidth = 75
)

// ExportVCard renders the user's contacts as a single vCard 3.0 stream, in the
// same name order as the contact list. Contacts the pilot has flown with carry
// their crew roles as CATEGORIES.
func (s *ContactService) ExportVCard(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	contacts, err := s.contactRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil, err
	}

	// Roles are optional; the export proceeds without them.
	roles, err := s.contactRepo.RolesByContact(ctx, userID)
	if err != nil {
		roles = nil
	}

	var b strings.Builder
	for _, c := range contacts {
		if c == nil {
			continue
		}
		writeVCard(&b, c, roles[c.ID])
	}
	return []byte(b.String()), nil
}

func writeVCard(b *strings.Builder, c *models.Contact, roles []string) {
	line := func(name, value string) {
		if value == "" {
			return
		}
		writeVCardLine(b, name+":"+vcardEscape(value))
	}

	writeVCardLine(b, "BEGIN:VCARD")
	writeVCardLine(b, "VERSION:3.0")

	// FN carries the display name; N is emitted from a best-effort split.
	line("FN", c.Name)
	family, given := splitVCardName(c.Name)
	writeVCardLine(b, "N:"+vcardEscape(family)+";"+vcardEscape(given)+";;;")

	if c.Email != nil {
		line("EMAIL;TYPE=INTERNET", strings.TrimSpace(*c.Email))
	}
	if c.Phone != nil {
		line("TEL;TYPE=VOICE", strings.TrimSpace(*c.Phone))
	}
	if c.Notes != nil {
		line("NOTE", strings.TrimSpace(*c.Notes))
	}
	if len(roles) > 0 {
		// Each role is escaped on its own, then joined with a real comma.
		escaped := make([]string, 0, len(roles))
		for _, r := range roles {
			escaped = append(escaped, vcardEscape(r))
		}
		writeVCardLine(b, "CATEGORIES:"+strings.Join(escaped, ","))
	}

	// UID is stable across re-exports.
	writeVCardLine(b, "UID:urn:uuid:"+c.ID.String())
	writeVCardLine(b, "REV:"+c.UpdatedAt.UTC().Format("20060102T150405Z"))
	writeVCardLine(b, "END:VCARD")
}

// vcardEscape applies RFC 2426 §5 escaping, newlines included.
func vcardEscape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "\\n",
		",", "\\,",
		";", "\\;",
	)
	return r.Replace(s)
}

// writeVCardLine appends one content line, folded to vcardFoldWidth octets;
// the split point walks back off any UTF-8 continuation byte.
func writeVCardLine(b *strings.Builder, line string) {
	for len(line) > vcardFoldWidth {
		cut := vcardFoldWidth
		for cut > 1 && line[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString(vcardCRLF)
		b.WriteString(" ")
		line = line[cut:]
	}
	b.WriteString(line)
	b.WriteString(vcardCRLF)
}

// splitVCardName splits a free-text name into (family, given) for the N
// property; the last whitespace-separated token is taken as the family name.
func splitVCardName(name string) (family, given string) {
	fields := strings.Fields(name)
	switch len(fields) {
	case 0:
		return "", ""
	case 1:
		return fields[0], ""
	default:
		return fields[len(fields)-1], strings.Join(fields[:len(fields)-1], " ")
	}
}
