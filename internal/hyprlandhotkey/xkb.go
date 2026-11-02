package hyprlandhotkey

/*
#cgo pkg-config: xkbcommon
#include <xkbcommon/xkbcommon.h>
#include <stdlib.h>

static int runix_contains_key(struct xkb_keymap *map, xkb_keycode_t code, xkb_keysym_t target) {
    xkb_layout_index_t layouts = xkb_keymap_num_layouts_for_key(map, code);
    for (xkb_layout_index_t layout = 0; layout < layouts; layout++) {
        xkb_level_index_t levels = xkb_keymap_num_levels_for_key(map, code, layout);
        for (xkb_level_index_t level = 0; level < levels; level++) {
            const xkb_keysym_t *symbols = NULL;
            int count = xkb_keymap_key_get_syms_by_level(map, code, layout, level, &symbols);
            for (int i = 0; i < count; i++) {
                // NoSymbol as target asks whether this position has any usable symbol.
                if (symbols[i] != XKB_KEY_NoSymbol &&
                    (target == XKB_KEY_NoSymbol || symbols[i] == target)) return 1;
            }
        }
    }
    return 0;
}

static struct xkb_keymap *runix_keymap(struct xkb_context *ctx, const char *rules,
 const char *model, const char *layout, const char *variant, const char *options) {
    struct xkb_rule_names names = { rules, model, layout, variant, options };
    return xkb_keymap_new_from_names(ctx, &names, XKB_KEYMAP_COMPILE_NO_FLAGS);
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

type xkbLayout struct{ Rules, Model, Layout, Variant, Options string }

type xkbMapper struct {
	ctx  *C.struct_xkb_context
	maps []*C.struct_xkb_keymap
}

func newXKBMapper(layouts []xkbLayout) (*xkbMapper, bool) {
	ctx := C.xkb_context_new(C.XKB_CONTEXT_NO_FLAGS)
	if ctx == nil {
		return nil, false
	}
	mapper := &xkbMapper{ctx: ctx, maps: make([]*C.struct_xkb_keymap, 0, len(layouts))}
	seen := make(map[xkbLayout]struct{}, len(layouts))
	for _, layout := range layouts {
		if _, exists := seen[layout]; exists {
			continue
		}
		seen[layout] = struct{}{}
		values := []string{layout.Rules, layout.Model, layout.Layout, layout.Variant, layout.Options}
		cvalues := [5]*C.char{}
		for i, value := range values {
			if value != "" {
				cvalues[i] = C.CString(value)
			}
		}
		mapped := C.runix_keymap(ctx, cvalues[0], cvalues[1], cvalues[2], cvalues[3], cvalues[4])
		for _, value := range cvalues {
			if value != nil {
				C.free(unsafe.Pointer(value))
			}
		}
		if mapped == nil {
			mapper.close()
			return nil, false
		}
		mapper.maps = append(mapper.maps, mapped)
	}
	return mapper, true
}

func (m *xkbMapper) close() {
	for _, keymap := range m.maps {
		C.xkb_keymap_unref(keymap)
	}
	C.xkb_context_unref(m.ctx)
}

func (m *xkbMapper) hasKeycode(target uint32) bool {
	for _, keymap := range m.maps {
		if target >= uint32(C.xkb_keymap_min_keycode(keymap)) && target <= uint32(C.xkb_keymap_max_keycode(keymap)) &&
			C.runix_contains_key(keymap, C.xkb_keycode_t(target), C.XKB_KEY_NoSymbol) != 0 {
			return true
		}
	}
	return false
}

func (m *xkbMapper) matches(key string, code uint32) (bool, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, false
	}
	cname := C.CString(key)
	target := C.xkb_keysym_from_name(cname, C.XKB_KEYSYM_CASE_INSENSITIVE)
	C.free(unsafe.Pointer(cname))
	if target == C.XKB_KEY_NoSymbol {
		return false, false
	}
	for _, keymap := range m.maps {
		if C.runix_contains_key(keymap, C.xkb_keycode_t(code), target) != 0 {
			return true, true
		}
	}
	return false, true
}
