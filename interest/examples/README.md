# Example Interests

These files are **examples**, not built-in interests. The chat service ships
with **zero** interests loaded by default — each product is responsible for
providing its own interest catalog via `config.yaml`:

```yaml
interests:
  dirs:
    - /path/to/product/interests
```

The files here serve two purposes:

1. **Reference** for engineers and product owners writing new interests —
   they show the YAML frontmatter shape, the supported fields
   (`id`, `name`, `triggers`, `requires`, `output_target`), and how the
   markdown body is structured.
2. **Test fixtures** for the `interest` package's unit tests.

Do not assume these are loaded at runtime. If you want them, copy them into
your product's interests directory.
