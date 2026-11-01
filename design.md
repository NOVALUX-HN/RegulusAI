# REGULUS — Design System &amp; Visual Identity

**Document:** [`design.md`](http://design.md)  
**Status:** Approved Design Direction  
**Design System:** Obsidian &amp; Lion Gold  
**Brand:** REGULUS  
**Identity:** The Lion Star Harness

---

## 1. Design Philosophy

REGULUS is inspired by two complementary concepts:

1. **Regulus, the star of Leo:** orientation, leadership, brilliance and consistency.
2. **The Golden Lion:** strength, discipline, nobility, protection and authority.

These concepts define the visual identity of the project.

REGULUS must communicate the following qualities:

- Engineering precision.
- Reliability and trust.
- Controlled execution.
- Continuous evolution.
- Modularity and extensibility.
- Technical sophistication.

### Brand statement

> Discipline. Verification. Evolution.

### Core principle

REGULUS must look like serious engineering infrastructure, not another generic AI product.

Avoid excessive cyberpunk aesthetics, excessive gradients, decorative neon effects and unnecessary visual complexity.

The visual language should be elegant, restrained, technical and recognizable.

---

## 2. Brand Identity

### 2.1 Brand name

Primary name: `REGULUS`

Secondary descriptor: `THE LION STAR HARNESS`

The brand name must always be written in uppercase when used as the primary logo or identity heading.

### 2.2 Brand symbol

The preferred logo direction combines:

- A geometric four-point Regulus star.
- Abstract lion mane geometry.
- Astronomical symmetry.
- Clean, recognizable silhouettes.

The symbol must remain recognizable at small sizes.

It must work in:

- Application icons.
- CLI interfaces.
- Documentation.
- GitHub repository branding.
- Dashboard navigation.
- MCP integrations.
- Terminal environments.
- Favicons.

Do not reproduce existing copyrighted character designs, armor, illustrations or logos.

The lion inspiration must be original and abstract.

### 2.3 Logo variants

The design system should support:

- Primary logo: symbol + REGULUS wordmark.
- Symbol-only logo.
- Monochrome logo.
- Gold-on-dark logo.
- Light-background logo.
- Small-size icon.

Gold is the preferred brand color. Silver or white may be used when contrast requires it.

---

## 3. Color System

The official palette is **Obsidian &amp; Lion Gold**.

The system uses dark graphite surfaces, warm metallic gold and controlled complementary colors.

### 3.1 Primary brand colors


| Token               | Hex       | Purpose                      |
| ------------------- | --------- | ---------------------------- |
| `brand-gold`        | `#D9A441` | Primary brand identity       |
| `brand-gold-bright` | `#F2C66D` | Highlights and active states |
| `brand-amber`       | `#B96C32` | Warm secondary accent        |
| `brand-silver`      | `#E7EAF0` | Primary light text           |


### 3.2 Neutral colors


| Token            | Hex       | Purpose                     |
| ---------------- | --------- | --------------------------- |
| `bg-primary`     | `#0C1018` | Main application background |
| `bg-secondary`   | `#171E29` | Cards and panels            |
| `bg-tertiary`    | `#202A38` | Elevated surfaces           |
| `text-primary`   | `#E7EAF0` | Main text                   |
| `text-secondary` | `#8290A3` | Secondary text              |
| `border-subtle`  | `#303B4B` | Dividers and borders        |


### 3.3 Module identity colors

Each major REGULUS module has its own visual identity.


| Module                | Token                  | Hex       |
| --------------------- | ---------------------- | --------- |
| Sextant               | `module-sextant`       | `#5B9BD5` |
| Gauntlet              | `module-gauntlet`      | `#E69A42` |
| Guardians             | `module-guardians`     | `#55B889` |
| Decision Intelligence | `module-decision`      | `#A78BFA` |
| Almanac               | `module-almanac`       | `#48BFC0` |
| Evolution             | `module-evolution`     | `#F2C66D` |
| Observability         | `module-observability` | `#8290A3` |


Module colors are identifiers, not replacements for the global brand palette.

REGULUS itself remains gold.

### 3.4 Semantic status colors

Verdicts and operational statuses must use consistent semantic colors across the entire system.


| Status       | Token                 | Hex       |
| ------------ | --------------------- | --------- |
| PASS         | `status-pass`         | `#55B889` |
| FAIL         | `status-fail`         | `#E16B68` |
| BLOCKED      | `status-blocked`      | `#E69A42` |
| INCONCLUSIVE | `status-inconclusive` | `#8290A3` |


Important:

A verdict must always use its semantic color, regardless of which module generated it.

For example, a failed Evolution operation must display the FAIL color, not the Evolution gold.

Never communicate status using color alone. Include text labels and, where appropriate, icons.

---

## 4. Typography

The official typography system consists of three typefaces.

### 4.1 Headings — Space Grotesk

Usage:

- Main headings.
- Page titles.
- Section titles.
- Brand wordmark.
- Dashboard headings.

Preferred weights:

- 500 Medium.
- 600 SemiBold.
- 700 Bold.

### 4.2 Interface — Inter

Usage:

- Body text.
- Documentation.
- Navigation.
- Buttons.
- Forms.
- Descriptions.
- Tables.
- Tooltips.

Preferred weights:

- 400 Regular.
- 500 Medium.
- 600 SemiBold.

### 4.3 Technical content — JetBrains Mono

Usage:

- Source code.
- Terminal output.
- Logs.
- Identifiers.
- Configuration.
- JSON payloads.
- Technical metrics.
- Execution traces.

Preferred weights:

- 400 Regular.
- 500 Medium.

### 4.4 Typography rules

- Prioritize readability over decorative styling.
- Avoid excessive uppercase text in body content.
- Use consistent heading hierarchy.
- Use monospace typography for technical data.
- Do not mix additional typefaces without an explicit design-system decision.

Recommended loading order:

```
font-family: "Space Grotesk", sans-serif;
font-family: "Inter", sans-serif;
font-family: "JetBrains Mono", monospace;
```

---

## 5. Design Tokens

All interfaces and visual components must consume centralized design tokens.

Do not hardcode brand colors independently across components.

### 5.1 CSS implementation

```
:root {
  /* Brand */
  --regulus-brand: #D9A441;
  --regulus-brand-bright: #F2C66D;
  --regulus-brand-dark: #B96C32;

  /* Backgrounds */
  --regulus-bg: #0C1018;
  --regulus-surface: #171E29;
  --regulus-surface-raised: #202A38;

  /* Typography */
  --regulus-text: #E7EAF0;
  --regulus-muted: #8290A3;

  /* Borders */
  --regulus-border: #303B4B;

  /* Modules */
  --module-sextant: #5B9BD5;
  --module-gauntlet: #E69A42;
  --module-guardians: #55B889;
  --module-decision: #A78BFA;
  --module-almanac: #48BFC0;
  --module-evolution: #F2C66D;
  --module-observability: #8290A3;

  /* Semantic statuses */
  --status-pass: #55B889;
  --status-fail: #E16B68;
  --status-blocked: #E69A42;
  --status-inconclusive: #8290A3;

  /* Typography */
  --font-heading: "Space Grotesk", sans-serif;
  --font-body: "Inter", sans-serif;
  --font-mono: "JetBrains Mono", monospace;

  /* Shape */
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 12px;
  --radius-xl: 16px;

  /* Borders */
  --border-width: 1px;

  /* Motion */
  --transition-fast: 150ms ease;
  --transition-normal: 220ms ease;
}
```

### 5.2 Token ownership

The design system must have one canonical source of truth.

If multiple frontend packages, plugins or documentation applications exist, they must consume the same tokens or their generated equivalents.

Avoid duplicated and independently maintained palettes.

---

## 6. Interface Design Principles

### 6.1 Backgrounds

Use:

- Obsidian for the primary background.
- Graphite for panels.
- Raised graphite for active or elevated components.

Avoid pure black as the default background.

### 6.2 Cards and panels

Cards should use:

- Dark graphite surfaces.
- Subtle borders.
- Consistent spacing.
- Small or medium corner radii.
- Minimal shadows.

Gold borders should be reserved for selected, featured or important components.

### 6.3 Buttons

Primary buttons:

- Gold background.
- Dark foreground.
- Clear action labels.

Secondary buttons:

- Graphite background.
- Silver text.
- Subtle border.

Dangerous actions:

- Semantic failure color.
- Explicit labels.
- Confirmation where necessary.

### 6.4 Navigation

Navigation should be minimal and functional.

Use the module identity color for module-specific icons or small accents.

The primary REGULUS brand symbol should remain gold.

### 6.5 Tables and technical data

- Use Inter for labels and descriptions.
- Use JetBrains Mono for identifiers, hashes, timestamps and technical values.
- Align numerical values consistently.
- Preserve visual hierarchy between headers, values and metadata.

---

## 7. Module Visual Language

Each module must have a recognizable icon and color.


| Module                | Visual concept                       | Color        |
| --------------------- | ------------------------------------ | ------------ |
| Sextant               | Navigation, orientation and planning | Stellar Blue |
| Gauntlet              | Execution, challenge and validation  | Lion Amber   |
| Guardians             | Protection and verification          | Emerald      |
| Decision Intelligence | Reasoning and decisions              | Violet       |
| Almanac               | Knowledge and memory                 | Turquoise    |
| Evolution             | Growth and adaptation                | Solar Gold   |
| Observability         | Monitoring and telemetry             | Steel Gray   |


The icons should be:

- Geometric.
- Minimal.
- Consistent in stroke weight.
- Recognizable at small sizes.
- Suitable for both light and dark backgrounds.

Do not create a different icon style for each module.

---

## 8. Visualizing the Harness

REGULUS is an engineering harness, so its visual language must prioritize information architecture.

The following elements should have standardized visual representations:

### Execution

Display:

- Execution identifier.
- Current phase.
- Active agent.
- Progress.
- Execution duration.
- Relevant artifacts.

### Verification

Display:

- Verification level.
- Guardian responsible.
- Evidence.
- Checks performed.
- Final result.

### Verdicts

Use the official semantic status colors.

A verdict must be distinguishable from a module identity or decorative indicator.

### Memory

Display:

- Knowledge source.
- Provenance.
- Timestamp.
- Relationships.
- Confidence where applicable.
- Validation status.

### Evolution

Display:

- Baseline.
- Proposed change.
- Evaluation results.
- Regression results.
- Adoption or rejection decision.

The interface must never imply that an unverified result is verified.

---

## 9. CLI and Terminal Identity

The terminal is part of the REGULUS experience.

CLI output should follow the same identity system.

Recommended conventions:


| Element            | Presentation   |
| ------------------ | -------------- |
| REGULUS header     | Gold           |
| Module identifier  | Module color   |
| PASS               | Green          |
| FAIL               | Red            |
| BLOCKED            | Amber          |
| INCONCLUSIVE       | Steel gray     |
| Commands           | JetBrains Mono |
| Technical metadata | Muted gray     |


CLI output must remain readable in terminals that do not support color.

Use semantic labels and symbols in addition to ANSI colors.

Do not use excessive ASCII art, animations or decorative banners.

The CLI must prioritize clarity, accessibility and low visual noise.

---

## 10. Documentation and Diagrams

Documentation must follow the same visual identity.

### Documentation

- Dark charcoal backgrounds where supported.
- Gold for primary headings and important highlights.
- Inter for readable prose.
- JetBrains Mono for code.
- Module colors for architecture references.

### Architecture diagrams

Use:

- Gold for the REGULUS Core.
- Module colors for individual modules.
- Neutral gray for infrastructure.
- Clearly differentiated arrows for execution and data flows.
- Consistent iconography.
- Explicit labels.

Diagrams must communicate architecture, not merely decorate documentation.

### GitHub

The repository identity should use:

- Gold symbol.
- Dark background.
- Clear REGULUS wordmark.
- Restrained astronomical or lion-inspired visual elements.

---

## 11. Motion and Effects

Motion must communicate meaningful system activity.

Allowed:

- Subtle hover transitions.
- Smooth state changes.
- Progress indicators.
- Verification completion feedback.
- Carefully controlled active-state highlights.

Avoid:

- Constant pulsing.
- Excessive glow.
- Unnecessary particle effects.
- Decorative animations that distract from technical information.

Gold glow may be used sparingly for important brand moments.

Motion must respect reduced-motion preferences.

---

## 12. Accessibility

The design system must meet practical accessibility requirements.

Rules:

1. Maintain WCAG AA contrast wherever possible.
2. Never communicate critical information through color alone.
3. Include text labels for verdicts.
4. Provide visible keyboard focus.
5. Support keyboard navigation.
6. Respect reduced-motion preferences.
7. Avoid excessively small typography.
8. Ensure icons have accessible labels when they convey information.

The gold palette must be evaluated against the actual surface on which it appears. Do not assume every gold shade is suitable for small text.

---

## 13. Responsive Behavior

Interfaces must adapt to:

- Desktop.
- Laptop.
- Tablet.
- Mobile.

Prioritize information hierarchy over preserving desktop layouts at smaller sizes.

Tables may become stacked data views when necessary.

Navigation should collapse without hiding critical system status.

Technical information must remain accessible on small screens.

---

## 14. Implementation Rules for Agents

Any agent modifying the REGULUS interface, documentation or visual assets must follow this specification.

Before implementing visual changes:

1. Inspect the existing design system.
2. Identify the canonical token source.
3. Reuse existing components whenever possible.
4. Avoid introducing duplicate styling systems.
5. Follow the established typography hierarchy.
6. Apply module colors consistently.
7. Preserve semantic status colors.
8. Validate contrast and responsive behavior.

### Prohibited practices

- Introducing arbitrary brand colors.
- Using gold for every module.
- Replacing semantic verdict colors with module colors.
- Adding unrelated fonts.
- Creating inconsistent icon styles.
- Using excessive gradients or neon effects.
- Hardcoding colors when an existing token is available.
- Introducing visual dependencies without justification.
- Changing system behavior solely to accommodate decorative UI.

### Change management

If a new design requirement conflicts with this specification:

- Identify the conflict.
- Explain the reason for the proposed change.
- Update the canonical tokens or rules when justified.
- Ensure all affected components remain consistent.

Do not silently create a parallel design system.

---

## 15. Design System Acceptance Criteria

An implementation is considered compliant when:

- The official palette is centralized.
- Typography follows the approved hierarchy.
- All modules use their designated identity colors.
- Verdicts use semantic status colors.
- Components share consistent spacing, borders and surfaces.
- CLI output follows the same visual language.
- Documentation and diagrams are consistent.
- Accessibility has been considered.
- Responsive behavior has been validated.
- No unnecessary duplicated styling systems exist.

---

## 16. Final Brand Direction

**REGULUS — THE LION STAR HARNESS**

Obsidian &amp; Lion Gold.

A visual identity inspired by the star of Leo and the noble strength of the golden lion.

REGULUS represents disciplined execution, verifiable intelligence and continuous evolution.

The design system must reinforce one central idea:

> Strength through discipline. Intelligence through verification. Evolution through evidence.

This document is the canonical visual reference for all REGULUS interfaces, documentation, tools and integrations.

All future visual implementations should follow this specification unless an explicitly approved revision supersedes it.