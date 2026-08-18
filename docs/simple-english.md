# Use Simple English

The Simple English profile keeps commit messages direct and easy to scan. It is enabled by default.

A commit subject is an instruction, such as `Prevent duplicate invoice delivery`. The body describes the reason and important facts.

## Configure the profile

Add this section to `.prosecheck.json`:

```json
{
  "simpleEnglish": {
    "enabled": true,
    "severity": "warning",
    "allow": ["OAuth", "SAML", "webhook"]
  }
}
```

The severity can be `error`, `warning`, or `info`. The Git hook uses strict mode, so errors and warnings stop the commit.

Set `enabled` to `false` to turn off all Simple English rules. A rule entry can still change one rule.

```json
{
  "rules": {
    "PC017": "info",
    "PC018": "off"
  }
}
```

## Rules

The profile adds these checks:

| Rule | Problem | Example correction |
|---|---|---|
| `PC014` | Semicolon | Write two sentences. |
| `PC015` | Contraction | Replace `doesn't` with `does not`. |
| `PC016` | Wordy phrase | Replace `in order to` with `to`. |
| `PC017` | Uncertain modal verb | Replace `might` with `can`, or state the condition. |
| `PC018` | Latin abbreviation | Replace `e.g.` with `for example`. |
| `PC019` | Complex verb tense | Replace `has been removed` with a simple past or present form. |

The existing body limit keeps each descriptive sentence at 25 words or fewer.

## Protected text

Prosecheck does not inspect these items with the Simple English phrase rules:

- Text in backticks
- Text in double quotation marks
- Command flags, URLs, paths, file names, and common code files
- Terms in `simpleEnglish.allow`

Use backticks for identifiers and exact technical values. Add a product or project term to the allow list when it overlaps a rule.

## Scope

This profile uses practical structure from Simplified Technical English. It does not contain the official ASD-STE100 dictionary.

The profile cannot prove full ASD-STE100 compliance. It focuses on reliable rules that work well for commit subjects and bodies.

The optional model does not enforce these rules. Local checks give consistent results and can block a commit.
