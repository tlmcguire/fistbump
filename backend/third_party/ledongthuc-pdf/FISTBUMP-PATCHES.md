# Local patches

Vendored from github.com/ledongthuc/pdf v0.0.0-20260907135840-6c8c28e0e8a0 (BSD-3-Clause, see LICENSE).
`backend/go.mod` points the module here with a `replace` directive.

1. `page.go`, `cmap.Decode`: bfrange offsets were added to the last byte of the destination only.
   Codes above 0xFF in a range (for example the identity map `<0000> <FFFF> <0000>` that fpdf and many
   word processors write) decoded to the wrong character: U+2022 became U+0022, U+0141 became U+0041.
   The fix adds the full big-endian offset with carry (`beUint`, `addOffset`).
   Proposed upstream in https://github.com/ledongthuc/pdf/pull/95. Remove the replace directive once released.
