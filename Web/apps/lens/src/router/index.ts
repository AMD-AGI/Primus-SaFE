import { RouteRecordRaw, createRouter, createWebHistory } from "vue-router";
import setupAuthGuard from './authGuard'

const routes: RouteRecordRaw[] = [
    {
        path: '/',
        redirect: '/statistics/cluster',
    },
    {
        path: '/login',
        name: 'Login',
        component: () => import('@/pages/Login/index.vue'),
        meta: { requiresAuth: false }
    },
    {
        path: '/sso-bridge',
        name: 'SSOBridge',
        component: () => import('@/pages/Login/SSOBridge.vue'),
        meta: { requiresAuth: false }
    },
    {
        path: '/dashboard',
        name: 'Dashboard',
        component: () => import('@/pages/Dashboard/index.vue'),
    },
    {
        path: '/nodes',
        name: 'Nodes',
        component: () => import('@/pages/Nodes/index.vue'),
    },
    {
        path: '/nodedetail/:name?',
        name: 'NodeDetail',
        component: () => import('@/pages/Nodes/NodeDetail.vue'),
    },
    {
        path: '/workloads',
        name: 'Workloads',
        component: () => import('@/pages/Workloads/index.vue'),
    },
    {
        path: '/workload/detail',
        name: 'WorkloadDetail',
        component: () => import('@/pages/Workloads/WorkloadDetail.vue'),
    },
    {
        path: '/workloads/:workloadUid/tracelens/:sessionId',
        name: 'TraceLensAnalysis',
        component: () => import('@/pages/TraceLens/AnalysisPage.vue'),
    },
    {
        path: '/workloads/:workloadUid/perfetto/:fileId',
        name: 'PerfettoViewer',
        component: () => import('@/pages/Perfetto/ViewerPage.vue'),
    },
    {
        path: '/statistics/cluster',
        name: 'ClusterStatistics',
        component: () => import('@/pages/Statistics/ClusterStats.vue'),
    },
    {
        path: '/statistics/namespace',
        name: 'NamespaceStatistics',
        component: () => import('@/pages/Statistics/NamespaceStats.vue'),
    },
    {
        path: '/statistics/workload',
        name: 'WorkloadStatistics',
        component: () => import('@/pages/Statistics/WorkloadStats.vue'),
    },
    {
        path: '/statistics/label',
        name: 'LabelStatistics',
        component: () => import('@/pages/Statistics/LabelStats.vue'),
    },
    {
        path: '/statistics',
        redirect: '/statistics/cluster',
    },
    {
        path: '/weekly-reports',
        name: 'WeeklyReports',
        component: () => import('@/pages/WeeklyReports/index.vue'),
    },
    {
        path: '/weekly-reports/:id',
        name: 'WeeklyReportDetail',
        component: () => import('@/pages/WeeklyReports/ReportDetail.vue'),
    },
    {
        path: '/github-workflow',
        name: 'GithubWorkflow',
        component: () => import('@/pages/GithubWorkflow/index.vue'),
    },
    // Repository Detail Route (New - Primary)
    {
        path: '/github-workflow/repos/:owner/:repo',
        name: 'GithubWorkflowRepositoryDetail',
        component: () => import('@/pages/GithubWorkflow/RepositoryDetail.vue'),
    },
    // Runner Set Centric Routes (New - Primary)
    {
        path: '/github-workflow/runner-sets/:id',
        name: 'GithubWorkflowRunnerSetDetail',
        component: () => import('@/pages/GithubWorkflow/Detail.vue'),
        meta: { runnerSetCentric: true }
    },
    // Config-based route (Legacy - for backward compatibility)
    {
        path: '/github-workflow/configs/:id',
        name: 'GithubWorkflowConfigDetail',
        component: () => import('@/pages/GithubWorkflow/Detail.vue'),
        meta: { runnerSetCentric: false }
    },
    // Legacy route - redirect to runner-set-centric or config-based based on ID
    {
        path: '/github-workflow/:id',
        name: 'GithubWorkflowDetail',
        component: () => import('@/pages/GithubWorkflow/Detail.vue'),
        meta: { legacy: true }
    },
    // Run Detail Page - shows workflow run with GitHub Actions style topology
    {
        path: '/github-workflow/runs/:runId',
        name: 'GithubWorkflowRunDetail',
        component: () => import('@/pages/GithubWorkflow/RunDetail.vue'),
    },
    // Run Summary Detail Page - shows aggregated workflow run view
    {
        path: '/github-workflow/run-summary/:id',
        name: 'GithubWorkflowRunSummaryDetail',
        component: () => import('@/pages/GithubWorkflow/RunSummaryDetail.vue'),
    },
    {
        path: '/management',
        name: 'Management',
        component: () => import('@/pages/Management/index.vue'),
        redirect: '/management/job-history',
        children: [
            {
                path: 'job-history',
                name: 'JobExecutionHistory',
                component: () => import('@/pages/Management/JobExecutionHistory.vue'),
            },
            {
                path: 'detection-status',
                name: 'DetectionStatus',
                component: () => import('@/pages/Management/DetectionStatus.vue'),
            },
            {
                path: 'system-config',
                name: 'SystemConfig',
                component: () => import('@/pages/Management/SystemConfig.vue'),
            },
            {
                path: 'releases',
                name: 'ReleaseManagement',
                component: () => import('@/pages/Management/ReleaseManagement.vue'),
            },
            {
                path: 'clusters',
                name: 'ClusterManagement',
                component: () => import('@/pages/Management/ClusterManagement.vue'),
            },
        ]
    },
    {
        path: '/agent',
        name: 'Agent',
        component: () => import('@/pages/Agent/index.vue'),
    },
    // Alert System Routes
    {
        path: '/alerts',
        component: () => import('@/pages/Alerts/AlertsLayout.vue'),
        redirect: '/alerts/overview',
        children: [
            {
                path: 'overview',
                name: 'AlertOverview',
                component: () => import('@/pages/Alerts/AlertOverview.vue'),
            },
            {
                path: 'events',
                name: 'AlertEvents',
                component: () => import('@/pages/Alerts/AlertEventsList.vue'),
            },
            {
                path: 'events/:id',
                name: 'AlertEventDetail',
                component: () => import('@/pages/Alerts/AlertEventDetail.vue'),
            },
            {
                path: 'rules',
                redirect: '/alerts/rules/metric',
            },
            {
                path: 'rules/metric',
                name: 'MetricAlertRules',
                component: () => import('@/pages/Alerts/MetricAlertRules.vue'),
            },
            {
                path: 'rules/log',
                name: 'LogAlertRules',
                component: () => import('@/pages/Alerts/LogAlertRules.vue'),
            },
            {
                path: 'rules/templates',
                name: 'AlertRuleTemplates',
                component: () => import('@/pages/Alerts/AlertRuleTemplates.vue'),
            },
            {
                path: 'silences',
                name: 'AlertSilences',
                component: () => import('@/pages/Alerts/AlertSilences.vue'),
            },
            {
                path: 'advices',
                name: 'AlertAdvices',
                component: () => import('@/pages/Alerts/AlertAdvices.vue'),
            },
            {
                path: 'channels',
                name: 'NotificationChannels',
                component: () => import('@/pages/Alerts/NotificationChannels.vue'),
            },
        ]
    }
]

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
})

// Setup auth guard
setupAuthGuard(router)

export default router
