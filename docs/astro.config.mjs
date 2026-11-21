import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://amterp.github.io',
  base: '/tai/',
  integrations: [
    starlight({
      title: 'Tai',
      description: 'Manage AI agent configurations across targets',
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/amterp/tai',
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/amterp/tai/edit/main/docs/',
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Quick Start', slug: 'getting-started/quick-start' },
            { label: 'Config Repo Setup', slug: 'getting-started/config-repo-setup' },
          ],
        },
        {
          label: 'Core Concepts',
          items: [
            { label: 'How Tai Works', slug: 'concepts/how-tai-works' },
            { label: 'Assets', slug: 'concepts/assets' },
            { label: 'Bundles', slug: 'concepts/bundles' },
            { label: 'Profiles', slug: 'concepts/profiles' },
            { label: 'Sources', slug: 'concepts/sources' },
            { label: 'Patches', slug: 'concepts/patches' },
            { label: 'Targets', slug: 'concepts/targets' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Creating Personas, Delegates & Files', slug: 'guides/creating-assets' },
            { label: 'Creating Skills', slug: 'guides/creating-skills' },
            { label: 'Composing Bundles', slug: 'guides/composing-bundles' },
            { label: 'Working with Multiple Sources', slug: 'guides/multiple-sources' },
            { label: 'Patching Imported Assets', slug: 'guides/patching-imported-assets' },
            { label: 'Templating', slug: 'guides/templating' },
            { label: 'Target-Aware Assets', slug: 'guides/target-aware-assets' },
            { label: 'Switching & Managing Profiles', slug: 'guides/managing-profiles' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI Reference', slug: 'reference/cli' },
            { label: 'tai.yaml', slug: 'reference/tai-yaml' },
            { label: 'Bundle Format', slug: 'reference/bundle-format' },
            { label: 'Profile Format', slug: 'reference/profile-format' },
            { label: 'Asset Formats', slug: 'reference/asset-formats' },
            { label: 'Frontmatter', slug: 'reference/frontmatter' },
            { label: 'Template Syntax', slug: 'reference/template-syntax' },
            { label: 'Skill Includes', slug: 'reference/skill-includes' },
            { label: 'Config Assets (StatusLines)', slug: 'reference/config-assets' },
            { label: 'Installation Behavior', slug: 'reference/installation-behavior' },
          ],
        },
        {
          label: 'Targets',
          items: [
            { label: 'Claude Code', slug: 'targets/claude-code' },
            { label: 'Codex', slug: 'targets/codex' },
            { label: 'OpenCode', slug: 'targets/opencode' },
            { label: 'Antigravity', slug: 'targets/antigravity' },
          ],
        },
      ],
    }),
  ],
});
