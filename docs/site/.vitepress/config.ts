import { defineConfig } from 'vitepress'
import { loadTheme } from 'shiki'
import { join } from 'path'
import shiki from 'shiki'

export default async () => {
    let theme = await loadTheme(join(__dirname, './theme.json'))
    return defineConfig({
        lang: 'en-US',
        title: 'Kuberik',
        description: 'Kubernetes Continous Development system',
        markdown: {
            theme,
        },
        lastUpdated: false,
        themeConfig: {
            logo: "/logo.svg",
            nav: [
                { text: 'Guide', link: '/guide/getting-started' },
                { text: 'API Reference', link: '/api-reference/' },
            ],
            sidebar: [
                {
                    text: 'Introduction',
                    items: [
                        { text: 'What is Kuberik?', link: '/guide/what-is-kuberik' },
                        { text: 'Getting Started', link: '/guide/getting-started' },
                    ]
                }, {
                    text: 'Continous Delivery',
                    items: [
                        { 'text': 'Continous Integration', link: '/guide/continous-integration' },
                        { 'text': 'Continous Deployments', link: '/guide/continous-deployments' },
                        { 'text': 'Canary Rollouts', link: '/guide/canary' },
                        { 'text': 'Environment Promotion', link: '/guide/environments' },
                    ]
                }, {
                    text: 'Reference',
                    items: [
                        { 'text': 'Contributing', link: '/contributing' },
                        { 'text': 'Roadmap', link: '/roadmap' },
                        { 'text': 'Contact', link: '/contact' },
                    ]
                }
            ],
            editLink: {
                pattern: 'https://github.com/kuberik/kuberik/edit/main/docs/site/:path',
                text: 'Edit this page on GitHub'
            },
            footer: {
                message: 'Released under the Apache-2.0 License.',
                copyright: `Copyright © ${new Date().getFullYear()} Kuberik Authors`
            },
            socialLinks: [
                { icon: 'github', link: 'https://github.com/kuberik/kuberik' },
            ]
        }
    })
}
