// .vitepress/config.ts
import { defineConfig } from "file:///Volumes/SanDisk1Tb/MEGAFLOYD/floyd-code/node_modules/.pnpm/vitepress@1.6.4_@algolia+client-search@5.52.1_@types+node@22.19.17_fuse.js@7.5.0_lightn_6b899cb4d2befbeed33923b6a0bdccdc/node_modules/vitepress/dist/node/index.js";
import { withMermaid } from "file:///Volumes/SanDisk1Tb/MEGAFLOYD/floyd-code/node_modules/.pnpm/vitepress-plugin-mermaid@2.0.17_mermaid@11.15.0_vitepress@1.6.4_@algolia+client-search@_365622323dd833ceede40470cb4f129e/node_modules/vitepress-plugin-mermaid/dist/vitepress-plugin-mermaid.es.mjs";
import llmstxt from "file:///Volumes/SanDisk1Tb/MEGAFLOYD/floyd-code/node_modules/.pnpm/vitepress-plugin-llms@1.13.1/node_modules/vitepress-plugin-llms/dist/index.js";
var rawBase = process.env.VITEPRESS_BASE;
var base = rawBase ? rawBase.startsWith("/") ? rawBase.endsWith("/") ? rawBase : `${rawBase}/` : `/${rawBase}/` : "/";
var mermaidOptimizeDeps = [
  "@braintree/sanitize-url",
  "dayjs",
  "debug",
  "cytoscape-cose-bilkent",
  "cytoscape"
];
var config = withMermaid(defineConfig({
  base,
  title: "Floyd Code CLI Docs",
  description: "Floyd Code CLI Documentation",
  head: [
    ["link", { rel: "icon", type: "image/x-icon", href: `${base}favicon.ico` }],
    ["meta", { name: "theme-color", content: "#0a7aff" }]
  ],
  srcExclude: ["AGENTS.md", "superpowers/**"],
  locales: {
    zh: {
      label: "\u7B80\u4F53\u4E2D\u6587",
      lang: "zh-CN",
      link: "/zh/",
      title: "Floyd Code CLI \u6587\u6863",
      description: "Floyd Code CLI \u7528\u6237\u6587\u6863",
      themeConfig: {
        nav: [
          { text: "\u6307\u5357", link: "/zh/guides/getting-started", activeMatch: "/zh/guides/" },
          { text: "\u5B9A\u5236\u5316", link: "/zh/customization/mcp", activeMatch: "/zh/customization/" },
          { text: "\u914D\u7F6E", link: "/zh/configuration/config-files", activeMatch: "/zh/configuration/" },
          { text: "\u53C2\u8003\u624B\u518C", link: "/zh/reference/floyd-command", activeMatch: "/zh/reference/" },
          { text: "\u53D1\u5E03\u8BF4\u660E", link: "/zh/release-notes/changelog", activeMatch: "/zh/release-notes/" }
        ],
        sidebar: {
          "/zh/guides/": [
            {
              text: "\u6307\u5357",
              items: [
                { text: "\u5F00\u59CB\u4F7F\u7528", link: "/zh/guides/getting-started" },
                { text: "\u4ECE floyd-cli \u8FC1\u79FB", link: "/zh/guides/migration" },
                { text: "\u5E38\u89C1\u4F7F\u7528\u6848\u4F8B", link: "/zh/guides/use-cases" },
                { text: "\u4EA4\u4E92\u4E0E\u8F93\u5165", link: "/zh/guides/interaction" },
                { text: "\u4F1A\u8BDD\u4E0E\u4E0A\u4E0B\u6587", link: "/zh/guides/sessions" },
                { text: "\u5728 IDE \u4E2D\u4F7F\u7528", link: "/zh/guides/ides" },
                { text: "\u5728\u7F51\u9875\u4E2D\u4F7F\u7528", link: "/zh/guides/web" },
                { text: "\u8FDC\u7A0B\u63A7\u5236", link: "/zh/guides/remote-control" }
              ]
            }
          ],
          "/zh/customization/": [
            {
              text: "\u5B9A\u5236\u5316",
              items: [
                { text: "Model Context Protocol", link: "/zh/customization/mcp" },
                { text: "Agent Skills", link: "/zh/customization/skills" },
                { text: "Plugins", link: "/zh/customization/plugins" },
                { text: "Agent \u4E0E subagent", link: "/zh/customization/agents" },
                { text: "Hooks", link: "/zh/customization/hooks" },
                { text: "\u81EA\u5B9A\u4E49\u4E3B\u9898", link: "/zh/customization/themes" }
              ]
            }
          ],
          "/zh/configuration/": [
            {
              text: "\u914D\u7F6E",
              items: [
                { text: "\u914D\u7F6E\u6587\u4EF6", link: "/zh/configuration/config-files" },
                { text: "\u5E73\u53F0\u4E0E\u6A21\u578B", link: "/zh/configuration/providers" },
                { text: "\u914D\u7F6E\u8986\u76D6", link: "/zh/configuration/overrides" },
                { text: "\u73AF\u5883\u53D8\u91CF", link: "/zh/configuration/env-vars" },
                { text: "\u6570\u636E\u8DEF\u5F84", link: "/zh/configuration/data-locations" }
              ]
            }
          ],
          "/zh/reference/": [
            {
              text: "\u53C2\u8003\u624B\u518C",
              items: [
                { text: "f7 \u547D\u4EE4", link: "/zh/reference/floyd-command" },
                { text: "f7 acp \u5B50\u547D\u4EE4", link: "/zh/reference/floyd-acp" },
                { text: "\u670D\u52A1 API", link: "/zh/reference/server-api" },
                { text: "\u5185\u7F6E\u5DE5\u5177", link: "/zh/reference/tools" },
                { text: "\u659C\u6760\u547D\u4EE4", link: "/zh/reference/slash-commands" },
                { text: "\u952E\u76D8\u5FEB\u6377\u952E", link: "/zh/reference/keyboard" }
              ]
            }
          ],
          "/zh/release-notes/": [
            {
              text: "\u53D1\u5E03\u8BF4\u660E",
              items: [
                { text: "\u53D8\u66F4\u8BB0\u5F55", link: "/zh/release-notes/changelog" }
              ]
            }
          ]
        }
      }
    },
    en: {
      label: "English",
      lang: "en-US",
      link: "/en/",
      title: "Floyd Code CLI Docs",
      description: "Floyd Code CLI User Documentation",
      themeConfig: {
        nav: [
          { text: "Guides", link: "/en/guides/getting-started", activeMatch: "/en/guides/" },
          { text: "Customization", link: "/en/customization/mcp", activeMatch: "/en/customization/" },
          { text: "Configuration", link: "/en/configuration/config-files", activeMatch: "/en/configuration/" },
          { text: "Reference", link: "/en/reference/floyd-command", activeMatch: "/en/reference/" },
          { text: "Release Notes", link: "/en/release-notes/changelog", activeMatch: "/en/release-notes/" }
        ],
        sidebar: {
          "/en/guides/": [
            {
              text: "Guides",
              items: [
                { text: "Getting Started", link: "/en/guides/getting-started" },
                { text: "Migrating from floyd-cli", link: "/en/guides/migration" },
                { text: "Common Use Cases", link: "/en/guides/use-cases" },
                { text: "Interaction and Input", link: "/en/guides/interaction" },
                { text: "Sessions and Context", link: "/en/guides/sessions" },
                { text: "Using in IDEs", link: "/en/guides/ides" },
                { text: "Using Floyd Code in the browser", link: "/en/guides/web" },
                { text: "Remote Control", link: "/en/guides/remote-control" }
              ]
            }
          ],
          "/en/customization/": [
            {
              text: "Customization",
              items: [
                { text: "Model Context Protocol", link: "/en/customization/mcp" },
                { text: "Agent Skills", link: "/en/customization/skills" },
                { text: "Plugins", link: "/en/customization/plugins" },
                { text: "Agents and Subagents", link: "/en/customization/agents" },
                { text: "Hooks", link: "/en/customization/hooks" },
                { text: "Custom Themes", link: "/en/customization/themes" }
              ]
            }
          ],
          "/en/configuration/": [
            {
              text: "Configuration",
              items: [
                { text: "Config Files", link: "/en/configuration/config-files" },
                { text: "Providers and Models", link: "/en/configuration/providers" },
                { text: "Config Overrides", link: "/en/configuration/overrides" },
                { text: "Environment Variables", link: "/en/configuration/env-vars" },
                { text: "Data Locations", link: "/en/configuration/data-locations" }
              ]
            }
          ],
          "/en/reference/": [
            {
              text: "Reference",
              items: [
                { text: "f7 Command", link: "/en/reference/floyd-command" },
                { text: "f7 acp Subcommand", link: "/en/reference/floyd-acp" },
                { text: "Server API", link: "/en/reference/server-api" },
                { text: "Built-in Tools", link: "/en/reference/tools" },
                { text: "Slash Commands", link: "/en/reference/slash-commands" },
                { text: "Keyboard Shortcuts", link: "/en/reference/keyboard" }
              ]
            }
          ],
          "/en/release-notes/": [
            {
              text: "Release Notes",
              items: [
                { text: "Changelog", link: "/en/release-notes/changelog" }
              ]
            }
          ]
        }
      }
    }
  },
  themeConfig: {
    outline: [2, 3],
    search: { provider: "local" },
    socialLinks: [
      { icon: "github", link: "https://github.com/CaptainPhantasy/f7" }
    ]
  },
  vite: {
    optimizeDeps: {
      include: mermaidOptimizeDeps.map((dep) => `mermaid > ${dep}`)
    },
    plugins: [llmstxt()]
  }
}));
if (config.vite?.optimizeDeps?.include) {
  config.vite.optimizeDeps.include = config.vite.optimizeDeps.include.filter(
    (dep) => !mermaidOptimizeDeps.includes(dep)
  );
}
var config_default = config;
export {
  config_default as default
};
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiLnZpdGVwcmVzcy9jb25maWcudHMiXSwKICAic291cmNlc0NvbnRlbnQiOiBbImNvbnN0IF9fdml0ZV9pbmplY3RlZF9vcmlnaW5hbF9kaXJuYW1lID0gXCIvVm9sdW1lcy9TYW5EaXNrMVRiL01FR0FGTE9ZRC9mbG95ZC1jb2RlL2RvY3MvLnZpdGVwcmVzc1wiO2NvbnN0IF9fdml0ZV9pbmplY3RlZF9vcmlnaW5hbF9maWxlbmFtZSA9IFwiL1ZvbHVtZXMvU2FuRGlzazFUYi9NRUdBRkxPWUQvZmxveWQtY29kZS9kb2NzLy52aXRlcHJlc3MvY29uZmlnLnRzXCI7Y29uc3QgX192aXRlX2luamVjdGVkX29yaWdpbmFsX2ltcG9ydF9tZXRhX3VybCA9IFwiZmlsZTovLy9Wb2x1bWVzL1NhbkRpc2sxVGIvTUVHQUZMT1lEL2Zsb3lkLWNvZGUvZG9jcy8udml0ZXByZXNzL2NvbmZpZy50c1wiO2ltcG9ydCB7IGRlZmluZUNvbmZpZyB9IGZyb20gJ3ZpdGVwcmVzcydcbmltcG9ydCB7IHdpdGhNZXJtYWlkIH0gZnJvbSAndml0ZXByZXNzLXBsdWdpbi1tZXJtYWlkJ1xuaW1wb3J0IGxsbXN0eHQgZnJvbSAndml0ZXByZXNzLXBsdWdpbi1sbG1zJ1xuXG5jb25zdCByYXdCYXNlID0gcHJvY2Vzcy5lbnYuVklURVBSRVNTX0JBU0VcbmNvbnN0IGJhc2UgPSByYXdCYXNlXG4gID8gcmF3QmFzZS5zdGFydHNXaXRoKCcvJylcbiAgICA/IHJhd0Jhc2UuZW5kc1dpdGgoJy8nKSA/IHJhd0Jhc2UgOiBgJHtyYXdCYXNlfS9gXG4gICAgOiBgLyR7cmF3QmFzZX0vYFxuICA6ICcvJ1xuXG5jb25zdCBtZXJtYWlkT3B0aW1pemVEZXBzID0gW1xuICAnQGJyYWludHJlZS9zYW5pdGl6ZS11cmwnLFxuICAnZGF5anMnLFxuICAnZGVidWcnLFxuICAnY3l0b3NjYXBlLWNvc2UtYmlsa2VudCcsXG4gICdjeXRvc2NhcGUnLFxuXVxuXG5jb25zdCBjb25maWcgPSB3aXRoTWVybWFpZChkZWZpbmVDb25maWcoe1xuICBiYXNlLFxuICB0aXRsZTogJ0Zsb3lkIENvZGUgQ0xJIERvY3MnLFxuICBkZXNjcmlwdGlvbjogJ0Zsb3lkIENvZGUgQ0xJIERvY3VtZW50YXRpb24nLFxuXG4gIGhlYWQ6IFtcbiAgICBbJ2xpbmsnLCB7IHJlbDogJ2ljb24nLCB0eXBlOiAnaW1hZ2UveC1pY29uJywgaHJlZjogYCR7YmFzZX1mYXZpY29uLmljb2AgfV0sXG4gICAgWydtZXRhJywgeyBuYW1lOiAndGhlbWUtY29sb3InLCBjb250ZW50OiAnIzBhN2FmZicgfV0sXG4gIF0sXG5cbiAgc3JjRXhjbHVkZTogWydBR0VOVFMubWQnLCAnc3VwZXJwb3dlcnMvKionXSxcblxuICBsb2NhbGVzOiB7XG4gICAgemg6IHtcbiAgICAgIGxhYmVsOiAnXHU3QjgwXHU0RjUzXHU0RTJEXHU2NTg3JyxcbiAgICAgIGxhbmc6ICd6aC1DTicsXG4gICAgICBsaW5rOiAnL3poLycsXG4gICAgICB0aXRsZTogJ0Zsb3lkIENvZGUgQ0xJIFx1NjU4N1x1Njg2MycsXG4gICAgICBkZXNjcmlwdGlvbjogJ0Zsb3lkIENvZGUgQ0xJIFx1NzUyOFx1NjIzN1x1NjU4N1x1Njg2MycsXG4gICAgICB0aGVtZUNvbmZpZzoge1xuICAgICAgICBuYXY6IFtcbiAgICAgICAgICB7IHRleHQ6ICdcdTYzMDdcdTUzNTcnLCBsaW5rOiAnL3poL2d1aWRlcy9nZXR0aW5nLXN0YXJ0ZWQnLCBhY3RpdmVNYXRjaDogJy96aC9ndWlkZXMvJyB9LFxuICAgICAgICAgIHsgdGV4dDogJ1x1NUI5QVx1NTIzNlx1NTMxNicsIGxpbms6ICcvemgvY3VzdG9taXphdGlvbi9tY3AnLCBhY3RpdmVNYXRjaDogJy96aC9jdXN0b21pemF0aW9uLycgfSxcbiAgICAgICAgICB7IHRleHQ6ICdcdTkxNERcdTdGNkUnLCBsaW5rOiAnL3poL2NvbmZpZ3VyYXRpb24vY29uZmlnLWZpbGVzJywgYWN0aXZlTWF0Y2g6ICcvemgvY29uZmlndXJhdGlvbi8nIH0sXG4gICAgICAgICAgeyB0ZXh0OiAnXHU1M0MyXHU4MDAzXHU2MjRCXHU1MThDJywgbGluazogJy96aC9yZWZlcmVuY2UvZmxveWQtY29tbWFuZCcsIGFjdGl2ZU1hdGNoOiAnL3poL3JlZmVyZW5jZS8nIH0sXG4gICAgICAgICAgeyB0ZXh0OiAnXHU1M0QxXHU1RTAzXHU4QkY0XHU2NjBFJywgbGluazogJy96aC9yZWxlYXNlLW5vdGVzL2NoYW5nZWxvZycsIGFjdGl2ZU1hdGNoOiAnL3poL3JlbGVhc2Utbm90ZXMvJyB9LFxuICAgICAgICBdLFxuICAgICAgICBzaWRlYmFyOiB7XG4gICAgICAgICAgJy96aC9ndWlkZXMvJzogW1xuICAgICAgICAgICAge1xuICAgICAgICAgICAgICB0ZXh0OiAnXHU2MzA3XHU1MzU3JyxcbiAgICAgICAgICAgICAgaXRlbXM6IFtcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdcdTVGMDBcdTU5Q0JcdTRGN0ZcdTc1MjgnLCBsaW5rOiAnL3poL2d1aWRlcy9nZXR0aW5nLXN0YXJ0ZWQnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU0RUNFIGZsb3lkLWNsaSBcdThGQzFcdTc5RkInLCBsaW5rOiAnL3poL2d1aWRlcy9taWdyYXRpb24nIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU1RTM4XHU4OUMxXHU0RjdGXHU3NTI4XHU2ODQ4XHU0RjhCJywgbGluazogJy96aC9ndWlkZXMvdXNlLWNhc2VzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1x1NEVBNFx1NEU5Mlx1NEUwRVx1OEY5M1x1NTE2NScsIGxpbms6ICcvemgvZ3VpZGVzL2ludGVyYWN0aW9uJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1x1NEYxQVx1OEJERFx1NEUwRVx1NEUwQVx1NEUwQlx1NjU4NycsIGxpbms6ICcvemgvZ3VpZGVzL3Nlc3Npb25zJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1x1NTcyOCBJREUgXHU0RTJEXHU0RjdGXHU3NTI4JywgbGluazogJy96aC9ndWlkZXMvaWRlcycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdcdTU3MjhcdTdGNTFcdTk4NzVcdTRFMkRcdTRGN0ZcdTc1MjgnLCBsaW5rOiAnL3poL2d1aWRlcy93ZWInIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU4RkRDXHU3QTBCXHU2M0E3XHU1MjM2JywgbGluazogJy96aC9ndWlkZXMvcmVtb3RlLWNvbnRyb2wnIH0sXG4gICAgICAgICAgICAgIF0sXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgICAgJy96aC9jdXN0b21pemF0aW9uLyc6IFtcbiAgICAgICAgICAgIHtcbiAgICAgICAgICAgICAgdGV4dDogJ1x1NUI5QVx1NTIzNlx1NTMxNicsXG4gICAgICAgICAgICAgIGl0ZW1zOiBbXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnTW9kZWwgQ29udGV4dCBQcm90b2NvbCcsIGxpbms6ICcvemgvY3VzdG9taXphdGlvbi9tY3AnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnQWdlbnQgU2tpbGxzJywgbGluazogJy96aC9jdXN0b21pemF0aW9uL3NraWxscycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdQbHVnaW5zJywgbGluazogJy96aC9jdXN0b21pemF0aW9uL3BsdWdpbnMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnQWdlbnQgXHU0RTBFIHN1YmFnZW50JywgbGluazogJy96aC9jdXN0b21pemF0aW9uL2FnZW50cycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdIb29rcycsIGxpbms6ICcvemgvY3VzdG9taXphdGlvbi9ob29rcycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdcdTgxRUFcdTVCOUFcdTRFNDlcdTRFM0JcdTk4OTgnLCBsaW5rOiAnL3poL2N1c3RvbWl6YXRpb24vdGhlbWVzJyB9LFxuICAgICAgICAgICAgICBdLFxuICAgICAgICAgICAgfSxcbiAgICAgICAgICBdLFxuICAgICAgICAgICcvemgvY29uZmlndXJhdGlvbi8nOiBbXG4gICAgICAgICAgICB7XG4gICAgICAgICAgICAgIHRleHQ6ICdcdTkxNERcdTdGNkUnLFxuICAgICAgICAgICAgICBpdGVtczogW1xuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1x1OTE0RFx1N0Y2RVx1NjU4N1x1NEVGNicsIGxpbms6ICcvemgvY29uZmlndXJhdGlvbi9jb25maWctZmlsZXMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU1RTczXHU1M0YwXHU0RTBFXHU2QTIxXHU1NzhCJywgbGluazogJy96aC9jb25maWd1cmF0aW9uL3Byb3ZpZGVycycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdcdTkxNERcdTdGNkVcdTg5ODZcdTc2RDYnLCBsaW5rOiAnL3poL2NvbmZpZ3VyYXRpb24vb3ZlcnJpZGVzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1x1NzNBRlx1NTg4M1x1NTNEOFx1OTFDRicsIGxpbms6ICcvemgvY29uZmlndXJhdGlvbi9lbnYtdmFycycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdcdTY1NzBcdTYzNkVcdThERUZcdTVGODQnLCBsaW5rOiAnL3poL2NvbmZpZ3VyYXRpb24vZGF0YS1sb2NhdGlvbnMnIH0sXG4gICAgICAgICAgICAgIF0sXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgICAgJy96aC9yZWZlcmVuY2UvJzogW1xuICAgICAgICAgICAge1xuICAgICAgICAgICAgICB0ZXh0OiAnXHU1M0MyXHU4MDAzXHU2MjRCXHU1MThDJyxcbiAgICAgICAgICAgICAgaXRlbXM6IFtcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdmNyBcdTU0N0RcdTRFRTQnLCBsaW5rOiAnL3poL3JlZmVyZW5jZS9mbG95ZC1jb21tYW5kJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ2Y3IGFjcCBcdTVCNTBcdTU0N0RcdTRFRTQnLCBsaW5rOiAnL3poL3JlZmVyZW5jZS9mbG95ZC1hY3AnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU2NzBEXHU1MkExIEFQSScsIGxpbms6ICcvemgvcmVmZXJlbmNlL3NlcnZlci1hcGknIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU1MTg1XHU3RjZFXHU1REU1XHU1MTc3JywgbGluazogJy96aC9yZWZlcmVuY2UvdG9vbHMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU2NTlDXHU2NzYwXHU1NDdEXHU0RUU0JywgbGluazogJy96aC9yZWZlcmVuY2Uvc2xhc2gtY29tbWFuZHMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU5NTJFXHU3NkQ4XHU1RkVCXHU2Mzc3XHU5NTJFJywgbGluazogJy96aC9yZWZlcmVuY2Uva2V5Ym9hcmQnIH0sXG4gICAgICAgICAgICAgIF0sXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgICAgJy96aC9yZWxlYXNlLW5vdGVzLyc6IFtcbiAgICAgICAgICAgIHtcbiAgICAgICAgICAgICAgdGV4dDogJ1x1NTNEMVx1NUUwM1x1OEJGNFx1NjYwRScsXG4gICAgICAgICAgICAgIGl0ZW1zOiBbXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnXHU1M0Q4XHU2NkY0XHU4QkIwXHU1RjU1JywgbGluazogJy96aC9yZWxlYXNlLW5vdGVzL2NoYW5nZWxvZycgfSxcbiAgICAgICAgICAgICAgXSxcbiAgICAgICAgICAgIH0sXG4gICAgICAgICAgXSxcbiAgICAgICAgfSxcbiAgICAgIH0sXG4gICAgfSxcbiAgICBlbjoge1xuICAgICAgbGFiZWw6ICdFbmdsaXNoJyxcbiAgICAgIGxhbmc6ICdlbi1VUycsXG4gICAgICBsaW5rOiAnL2VuLycsXG4gICAgICB0aXRsZTogJ0Zsb3lkIENvZGUgQ0xJIERvY3MnLFxuICAgICAgZGVzY3JpcHRpb246ICdGbG95ZCBDb2RlIENMSSBVc2VyIERvY3VtZW50YXRpb24nLFxuICAgICAgdGhlbWVDb25maWc6IHtcbiAgICAgICAgbmF2OiBbXG4gICAgICAgICAgeyB0ZXh0OiAnR3VpZGVzJywgbGluazogJy9lbi9ndWlkZXMvZ2V0dGluZy1zdGFydGVkJywgYWN0aXZlTWF0Y2g6ICcvZW4vZ3VpZGVzLycgfSxcbiAgICAgICAgICB7IHRleHQ6ICdDdXN0b21pemF0aW9uJywgbGluazogJy9lbi9jdXN0b21pemF0aW9uL21jcCcsIGFjdGl2ZU1hdGNoOiAnL2VuL2N1c3RvbWl6YXRpb24vJyB9LFxuICAgICAgICAgIHsgdGV4dDogJ0NvbmZpZ3VyYXRpb24nLCBsaW5rOiAnL2VuL2NvbmZpZ3VyYXRpb24vY29uZmlnLWZpbGVzJywgYWN0aXZlTWF0Y2g6ICcvZW4vY29uZmlndXJhdGlvbi8nIH0sXG4gICAgICAgICAgeyB0ZXh0OiAnUmVmZXJlbmNlJywgbGluazogJy9lbi9yZWZlcmVuY2UvZmxveWQtY29tbWFuZCcsIGFjdGl2ZU1hdGNoOiAnL2VuL3JlZmVyZW5jZS8nIH0sXG4gICAgICAgICAgeyB0ZXh0OiAnUmVsZWFzZSBOb3RlcycsIGxpbms6ICcvZW4vcmVsZWFzZS1ub3Rlcy9jaGFuZ2Vsb2cnLCBhY3RpdmVNYXRjaDogJy9lbi9yZWxlYXNlLW5vdGVzLycgfSxcbiAgICAgICAgXSxcbiAgICAgICAgc2lkZWJhcjoge1xuICAgICAgICAgICcvZW4vZ3VpZGVzLyc6IFtcbiAgICAgICAgICAgIHtcbiAgICAgICAgICAgICAgdGV4dDogJ0d1aWRlcycsXG4gICAgICAgICAgICAgIGl0ZW1zOiBbXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnR2V0dGluZyBTdGFydGVkJywgbGluazogJy9lbi9ndWlkZXMvZ2V0dGluZy1zdGFydGVkJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ01pZ3JhdGluZyBmcm9tIGZsb3lkLWNsaScsIGxpbms6ICcvZW4vZ3VpZGVzL21pZ3JhdGlvbicgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdDb21tb24gVXNlIENhc2VzJywgbGluazogJy9lbi9ndWlkZXMvdXNlLWNhc2VzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ0ludGVyYWN0aW9uIGFuZCBJbnB1dCcsIGxpbms6ICcvZW4vZ3VpZGVzL2ludGVyYWN0aW9uJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1Nlc3Npb25zIGFuZCBDb250ZXh0JywgbGluazogJy9lbi9ndWlkZXMvc2Vzc2lvbnMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnVXNpbmcgaW4gSURFcycsIGxpbms6ICcvZW4vZ3VpZGVzL2lkZXMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnVXNpbmcgRmxveWQgQ29kZSBpbiB0aGUgYnJvd3NlcicsIGxpbms6ICcvZW4vZ3VpZGVzL3dlYicgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdSZW1vdGUgQ29udHJvbCcsIGxpbms6ICcvZW4vZ3VpZGVzL3JlbW90ZS1jb250cm9sJyB9LFxuICAgICAgICAgICAgICBdLFxuICAgICAgICAgICAgfSxcbiAgICAgICAgICBdLFxuICAgICAgICAgICcvZW4vY3VzdG9taXphdGlvbi8nOiBbXG4gICAgICAgICAgICB7XG4gICAgICAgICAgICAgIHRleHQ6ICdDdXN0b21pemF0aW9uJyxcbiAgICAgICAgICAgICAgaXRlbXM6IFtcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdNb2RlbCBDb250ZXh0IFByb3RvY29sJywgbGluazogJy9lbi9jdXN0b21pemF0aW9uL21jcCcgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdBZ2VudCBTa2lsbHMnLCBsaW5rOiAnL2VuL2N1c3RvbWl6YXRpb24vc2tpbGxzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1BsdWdpbnMnLCBsaW5rOiAnL2VuL2N1c3RvbWl6YXRpb24vcGx1Z2lucycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdBZ2VudHMgYW5kIFN1YmFnZW50cycsIGxpbms6ICcvZW4vY3VzdG9taXphdGlvbi9hZ2VudHMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnSG9va3MnLCBsaW5rOiAnL2VuL2N1c3RvbWl6YXRpb24vaG9va3MnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnQ3VzdG9tIFRoZW1lcycsIGxpbms6ICcvZW4vY3VzdG9taXphdGlvbi90aGVtZXMnIH0sXG4gICAgICAgICAgICAgIF0sXG4gICAgICAgICAgICB9LFxuICAgICAgICAgIF0sXG4gICAgICAgICAgJy9lbi9jb25maWd1cmF0aW9uLyc6IFtcbiAgICAgICAgICAgIHtcbiAgICAgICAgICAgICAgdGV4dDogJ0NvbmZpZ3VyYXRpb24nLFxuICAgICAgICAgICAgICBpdGVtczogW1xuICAgICAgICAgICAgICAgIHsgdGV4dDogJ0NvbmZpZyBGaWxlcycsIGxpbms6ICcvZW4vY29uZmlndXJhdGlvbi9jb25maWctZmlsZXMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnUHJvdmlkZXJzIGFuZCBNb2RlbHMnLCBsaW5rOiAnL2VuL2NvbmZpZ3VyYXRpb24vcHJvdmlkZXJzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ0NvbmZpZyBPdmVycmlkZXMnLCBsaW5rOiAnL2VuL2NvbmZpZ3VyYXRpb24vb3ZlcnJpZGVzJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ0Vudmlyb25tZW50IFZhcmlhYmxlcycsIGxpbms6ICcvZW4vY29uZmlndXJhdGlvbi9lbnYtdmFycycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdEYXRhIExvY2F0aW9ucycsIGxpbms6ICcvZW4vY29uZmlndXJhdGlvbi9kYXRhLWxvY2F0aW9ucycgfSxcbiAgICAgICAgICAgICAgXSxcbiAgICAgICAgICAgIH0sXG4gICAgICAgICAgXSxcbiAgICAgICAgICAnL2VuL3JlZmVyZW5jZS8nOiBbXG4gICAgICAgICAgICB7XG4gICAgICAgICAgICAgIHRleHQ6ICdSZWZlcmVuY2UnLFxuICAgICAgICAgICAgICBpdGVtczogW1xuICAgICAgICAgICAgICAgIHsgdGV4dDogJ2Y3IENvbW1hbmQnLCBsaW5rOiAnL2VuL3JlZmVyZW5jZS9mbG95ZC1jb21tYW5kJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ2Y3IGFjcCBTdWJjb21tYW5kJywgbGluazogJy9lbi9yZWZlcmVuY2UvZmxveWQtYWNwJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ1NlcnZlciBBUEknLCBsaW5rOiAnL2VuL3JlZmVyZW5jZS9zZXJ2ZXItYXBpJyB9LFxuICAgICAgICAgICAgICAgIHsgdGV4dDogJ0J1aWx0LWluIFRvb2xzJywgbGluazogJy9lbi9yZWZlcmVuY2UvdG9vbHMnIH0sXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnU2xhc2ggQ29tbWFuZHMnLCBsaW5rOiAnL2VuL3JlZmVyZW5jZS9zbGFzaC1jb21tYW5kcycgfSxcbiAgICAgICAgICAgICAgICB7IHRleHQ6ICdLZXlib2FyZCBTaG9ydGN1dHMnLCBsaW5rOiAnL2VuL3JlZmVyZW5jZS9rZXlib2FyZCcgfSxcbiAgICAgICAgICAgICAgXSxcbiAgICAgICAgICAgIH0sXG4gICAgICAgICAgXSxcbiAgICAgICAgICAnL2VuL3JlbGVhc2Utbm90ZXMvJzogW1xuICAgICAgICAgICAge1xuICAgICAgICAgICAgICB0ZXh0OiAnUmVsZWFzZSBOb3RlcycsXG4gICAgICAgICAgICAgIGl0ZW1zOiBbXG4gICAgICAgICAgICAgICAgeyB0ZXh0OiAnQ2hhbmdlbG9nJywgbGluazogJy9lbi9yZWxlYXNlLW5vdGVzL2NoYW5nZWxvZycgfSxcbiAgICAgICAgICAgICAgXSxcbiAgICAgICAgICAgIH0sXG4gICAgICAgICAgXSxcbiAgICAgICAgfSxcbiAgICAgIH0sXG4gICAgfSxcbiAgfSxcblxuICB0aGVtZUNvbmZpZzoge1xuICAgIG91dGxpbmU6IFsyLCAzXSxcbiAgICBzZWFyY2g6IHsgcHJvdmlkZXI6ICdsb2NhbCcgfSxcbiAgICBzb2NpYWxMaW5rczogW1xuICAgICAgeyBpY29uOiAnZ2l0aHViJywgbGluazogJ2h0dHBzOi8vZ2l0aHViLmNvbS9DYXB0YWluUGhhbnRhc3kvZjcnIH0sXG4gICAgXSxcbiAgfSxcblxuICB2aXRlOiB7XG4gICAgb3B0aW1pemVEZXBzOiB7XG4gICAgICBpbmNsdWRlOiBtZXJtYWlkT3B0aW1pemVEZXBzLm1hcCgoZGVwKSA9PiBgbWVybWFpZCA+ICR7ZGVwfWApLFxuICAgIH0sXG4gICAgcGx1Z2luczogW2xsbXN0eHQoKV0sXG4gIH0sXG59KSlcblxuaWYgKGNvbmZpZy52aXRlPy5vcHRpbWl6ZURlcHM/LmluY2x1ZGUpIHtcbiAgY29uZmlnLnZpdGUub3B0aW1pemVEZXBzLmluY2x1ZGUgPSBjb25maWcudml0ZS5vcHRpbWl6ZURlcHMuaW5jbHVkZS5maWx0ZXIoXG4gICAgKGRlcCkgPT4gIW1lcm1haWRPcHRpbWl6ZURlcHMuaW5jbHVkZXMoZGVwKSxcbiAgKVxufVxuXG5leHBvcnQgZGVmYXVsdCBjb25maWdcbiJdLAogICJtYXBwaW5ncyI6ICI7QUFBZ1YsU0FBUyxvQkFBb0I7QUFDN1csU0FBUyxtQkFBbUI7QUFDNUIsT0FBTyxhQUFhO0FBRXBCLElBQU0sVUFBVSxRQUFRLElBQUk7QUFDNUIsSUFBTSxPQUFPLFVBQ1QsUUFBUSxXQUFXLEdBQUcsSUFDcEIsUUFBUSxTQUFTLEdBQUcsSUFBSSxVQUFVLEdBQUcsT0FBTyxNQUM1QyxJQUFJLE9BQU8sTUFDYjtBQUVKLElBQU0sc0JBQXNCO0FBQUEsRUFDMUI7QUFBQSxFQUNBO0FBQUEsRUFDQTtBQUFBLEVBQ0E7QUFBQSxFQUNBO0FBQ0Y7QUFFQSxJQUFNLFNBQVMsWUFBWSxhQUFhO0FBQUEsRUFDdEM7QUFBQSxFQUNBLE9BQU87QUFBQSxFQUNQLGFBQWE7QUFBQSxFQUViLE1BQU07QUFBQSxJQUNKLENBQUMsUUFBUSxFQUFFLEtBQUssUUFBUSxNQUFNLGdCQUFnQixNQUFNLEdBQUcsSUFBSSxjQUFjLENBQUM7QUFBQSxJQUMxRSxDQUFDLFFBQVEsRUFBRSxNQUFNLGVBQWUsU0FB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
