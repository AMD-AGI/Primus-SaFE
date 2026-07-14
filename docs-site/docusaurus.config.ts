import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// Primus-SaFE documentation site.
// Docs-only mode: the docs tree is served at the site root ("/").

const GITHUB_REPO = 'https://github.com/AMD-AGI/Primus-SaFE';
const EDIT_BRANCH = 'ga-doc';

const config: Config = {
  title: 'Primus-SaFE',
  tagline: 'Stability at Scale: AMD’s Full-Stack Platform for Large-Model Training',
  favicon: 'img/logo.svg',

  // TODO(ga-doc): set to the real published URL before GA (e.g. GitHub Pages).
  url: 'https://amd-agi.github.io',
  baseUrl: '/Primus-SaFE/',

  organizationName: 'AMD-AGI',
  projectName: 'Primus-SaFE',

  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',

  markdown: {
    mermaid: true,
  },
  themes: ['@docusaurus/theme-mermaid'],

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: `${GITHUB_REPO}/tree/${EDIT_BRANCH}/docs-site/`,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/amd-primus-black.png',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'SaFE',
      logo: {
        alt: 'AMD Primus-SaFE',
        src: 'img/amd-primus-black.png',
        srcDark: 'img/amd-primus-white.png',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          // Versioning: once we cut the first release, run
          //   npm run docusaurus docs:version 1.0
          // and this dropdown will appear automatically.
          type: 'docsVersionDropdown',
          position: 'right',
        },
        {
          href: GITHUB_REPO,
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Overview', to: '/'},
            {label: 'Getting Started', to: '/getting-started/prerequisites'},
            {label: 'Concepts', to: '/concepts/workspace'},
            {label: 'Tasks', to: '/tasks/run-single-node-training'},
          ],
        },
        {
          title: 'Project',
          items: [
            {label: 'GitHub', href: GITHUB_REPO},
            {label: 'Issues', href: `${GITHUB_REPO}/issues`},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Advanced Micro Devices, Inc. Licensed under Apache 2.0.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go'],
    },
    // TODO(ga-doc): enable Algolia DocSearch before GA (free for OSS).
    // algolia: { appId: '...', apiKey: '...', indexName: 'primus-safe' },
  } satisfies Preset.ThemeConfig,
};

export default config;
