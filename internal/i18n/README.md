# Internalization catalogs

Product strings use `i18n.T` for the active process language or `i18n.TFor` when a component has an explicit locale. Catalogs are embedded at build time and merged from `<domain>.<language>.json` files, including `cli`, `doctor`, `onboarding`, `tui`, `backup`, `skills`, `brain`, `github`, `permissions`, `config`, and `prompts`.

Keep every new key in both languages and give each key one owner file per locale. `CatalogKeys` validates the merged key sets, and the i18n tests reject duplicate keys across embedded files.