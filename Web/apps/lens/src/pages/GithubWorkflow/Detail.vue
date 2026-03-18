<template>
  <div class="detail-page">
    <!-- Header -->
    <div class="page-header">
      <el-button @click="goBack" :icon="ArrowLeft">Back to Workflows</el-button>
      <h2 class="page-title" v-if="config || isRunnerSetMode">
        <template v-if="isRunnerSetCentric">
          {{ runnerSetInfo.name }}
          <template v-if="config">
            <el-tag type="success" effect="plain" style="margin-left: 12px;">
              {{ config.enabled ? 'Config Enabled' : 'Config Disabled' }}
            </el-tag>
          </template>
          <template v-else>
            <el-tag type="warning" effect="plain" style="margin-left: 12px;">No Config</el-tag>
            <el-button 
              type="primary" 
              size="small" 
              style="margin-left: 12px;"
              @click="showCreateConfigDialog = true"
            >
              <el-icon><Plus /></el-icon>
              Add Config
            </el-button>
          </template>
        </template>
        <template v-else-if="config">
          {{ config.name }}
          <el-tag :type="config.enabled ? 'success' : 'info'" effect="plain" style="margin-left: 12px;">
            {{ config.enabled ? 'Enabled' : 'Disabled' }}
          </el-tag>
        </template>
        <template v-else-if="isRunnerSetMode">
          {{ runnerSetInfo.name }}
          <el-tag type="warning" effect="plain" style="margin-left: 12px;">No Config</el-tag>
        </template>
      </h2>
    </div>
    
    <!-- Subtitle Info -->
    <div class="page-subtitle" v-if="config || isRunnerSetMode">
      <template v-if="config">
        <el-icon><Link /></el-icon>
        {{ config.githubOwner }}/{{ config.githubRepo }}
        <span class="separator">•</span>
        <el-icon><Box /></el-icon>
        {{ config.runnerSetName }}
      </template>
      <template v-else-if="isRunnerSetMode">
        <template v-if="runnerSetInfo.githubOwner && runnerSetInfo.githubRepo">
          <el-icon><Link /></el-icon>
          {{ runnerSetInfo.githubOwner }}/{{ runnerSetInfo.githubRepo }}
          <span class="separator">•</span>
        </template>
        <el-icon><Box /></el-icon>
        {{ runnerSetInfo.namespace }}
      </template>
    </div>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" class="detail-tabs">
      <!-- Runs Tab -->
      <el-tab-pane label="Runs" name="runs">
        <template #label>
          <span class="tab-label">
            <el-icon><List /></el-icon>
            Runs
            <el-badge v-if="runStats.pending > 0" :value="runStats.pending" class="tab-badge" />
          </span>
        </template>

        <div class="tab-content">
          <!-- Running Workflow Banner -->
          <transition name="banner-fade">
            <div v-if="runningRuns.length > 0" class="running-banner">
              <div class="banner-content">
                <span class="pulse-dot"></span>
                <span class="banner-text">
                  <strong>{{ runningRuns.length }}</strong> workflow{{ runningRuns.length > 1 ? 's' : '' }} currently running
                </span>
                <div class="running-items">
                  <el-tag 
                    v-for="run in runningRuns.slice(0, 3)" 
                    :key="run.id"
                    type="warning"
                    effect="light"
                    class="running-tag"
                    @click="goToRunDetail(run)"
                  >
                    {{ run.workloadName }}
                  </el-tag>
                  <el-tag v-if="runningRuns.length > 3" type="info" effect="plain" class="more-tag">
                    +{{ runningRuns.length - 3 }} more
                  </el-tag>
                </div>
              </div>
              <el-button type="primary" size="small" @click="goToRunDetail(runningRuns[0])">
                View Details
              </el-button>
            </div>
          </transition>

          <!-- Runs Card -->
          <el-card class="runs-card glass-card">
            <!-- Actions Bar -->
            <div class="filter-bar">
              <div class="flex-1"></div>
              <el-button 
                v-if="isRunnerSetCentric || !isLegacyRunnerSetMode" 
                type="primary" 
                :icon="Plus" 
                @click="showBackfillDialog = true" 
                size="default"
              >
                Backfill
              </el-button>
            </div>

          <!-- Runs Table -->
          <el-table 
            :data="runs" 
            v-loading="runsLoading" 
            stripe 
            style="width: 100%"
            @filter-change="handleFilterChange"
            @row-click="handleRowClick"
            :row-class-name="getRowClassName"
            class="clickable-table"
          >
            <el-table-column prop="workloadName" label="Workload" min-width="280" show-overflow-tooltip>
              <template #default="{ row }">
                <div class="workload-cell">
                  <span class="workload-name">{{ row.workloadName }}</span>
                  <span class="workload-ns">{{ row.workloadNamespace }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column 
              label="Workflow" 
              width="140" 
              align="center"
              prop="workflowStatus"
              column-key="workflowStatus"
              :filters="workflowStatusFilters"
            >
              <template #default="{ row }">
                <div class="status-cell">
                  <el-tag
                    :type="getWorkflowStatusType(row)"
                    :effect="isWorkflowRunning(row) ? 'dark' : 'light'"
                    size="small"
                    :class="{ 'is-running': isWorkflowRunning(row) }"
                  >
                    <span v-if="isWorkflowRunning(row)" class="running-dot"></span>
                    {{ getWorkflowStatusText(row) }}
                  </el-tag>
                  <span v-if="row.progressPercent > 0 && isWorkflowRunning(row)" class="progress-text">
                    {{ row.progressPercent }}%
                  </span>
                </div>
              </template>
            </el-table-column>
            <el-table-column 
              label="Collection" 
              width="120" 
              align="center"
              prop="collectionStatus"
              column-key="collectionStatus"
              :filters="collectionStatusFilters"
            >
              <template #default="{ row }">
                <el-tag
                  :type="getCollectionStatusType(row)"
                  effect="plain"
                  size="small"
                >
                  {{ getCollectionStatusText(row) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column 
              label="Trigger" 
              width="100" 
              align="center"
              prop="triggerSource"
              column-key="triggerSource"
              :filters="triggerFilters"
              :filtered-value="filteredTrigger"
              v-if="!isRunnerSetMode"
            >
              <template #default="{ row }">
                <el-tag type="info" effect="plain" size="small">
                  {{ row.triggerSource }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Workflow" width="120">
              <template #default="{ row }">
                <template v-if="row.githubRunId && hasGithubInfo">
                  <el-link
                    type="primary"
                    :href="getGithubRunUrl(row)"
                    target="_blank"
                    :underline="false"
                    class="workflow-link"
                  >
                    <el-icon><VideoPlay /></el-icon>
                    <span class="run-number">#{{ row.githubRunNumber || row.githubRunId }}</span>
                  </el-link>
                </template>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="Commit" width="160">
              <template #default="{ row }">
                <template v-if="row.headSha && hasGithubInfo">
                  <el-link
                    type="primary"
                    :href="getCommitUrl(row)"
                    target="_blank"
                    :underline="false"
                    class="commit-link"
                  >
                    <el-icon><Document /></el-icon>
                    <span class="commit-sha">{{ row.headSha.substring(0, 7) }}</span>
                  </el-link>
                  <span v-if="row.headBranch" class="commit-branch">{{ row.headBranch }}</span>
                </template>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="Duration" width="120">
              <template #default="{ row }">
                <span v-if="row.workloadStartedAt && isValidDate(row.workloadCompletedAt)">
                  {{ formatDuration(row.workloadStartedAt, row.workloadCompletedAt) }}
                </span>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="Completed At" width="160">
              <template #default="{ row }">
                <span v-if="row.workloadCompletedAt">{{ formatDate(row.workloadCompletedAt) }}</span>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="100" fixed="right" align="center">
              <template #default="{ row }">
                <el-button
                  link
                  type="primary"
                  @click="retryRun(row)"
                  :disabled="row.status !== 'failed'"
                >
                  Retry
                </el-button>
              </template>
            </el-table-column>
          </el-table>

          <!-- Pagination -->
          <div class="pagination-container">
            <el-pagination
              v-model:current-page="runsPagination.page"
              v-model:page-size="runsPagination.pageSize"
              :total="runsPagination.total"
              :page-sizes="[20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              @current-change="fetchRuns"
              @size-change="fetchRuns"
            />
          </div>
          </el-card>
        </div>
      </el-tab-pane>

      <!-- Analytics Tab (only with config) -->
      <el-tab-pane v-if="config" label="Analytics" name="analytics">
        <template #label>
          <span class="tab-label">
            <el-icon><DataAnalysis /></el-icon>
            Analytics
          </span>
        </template>

        <div class="tab-content">
          <!-- Analytics Sub Navigation -->
          <div class="analytics-nav">
            <el-radio-group v-model="analyticsView" size="default">
              <el-radio-button value="metrics">
                <el-icon><TrendCharts /></el-icon>
                Metrics
              </el-radio-button>
            </el-radio-group>
          </div>

          <!-- Metrics View -->
          <div v-show="analyticsView === 'metrics'" class="analytics-view">
          <!-- Query Builder -->
          <el-card class="query-card glass-card">
            <el-form :model="queryForm" label-width="100px" label-position="right">
              <!-- Schema Version Selector (multiple versions) -->
              <el-row :gutter="20" v-if="hasMultipleSchemas">
                <el-col :span="12">
                  <el-form-item label="Schema">
                    <el-select 
                      v-model="queryForm.schemaVersion" 
                      size="default" 
                      class="schema-version-select"
                      :loading="schemasLoading"
                    >
                      <el-option
                        v-for="s in schemaVersions"
                        :key="s.version"
                        :label="`v${s.version} (${formatNumber(s.recordCount)} records)`"
                        :value="s.version"
                      >
                        <div class="schema-option">
                          <span class="version-label">v{{ s.version }}</span>
                          <span class="record-count">{{ formatNumber(s.recordCount) }} records</span>
                          <span class="date-range">{{ formatDate(s.firstSeenAt) }}</span>
                          <el-tag v-if="s.isActive" type="success" size="small" effect="plain" class="active-tag">Active</el-tag>
                          <el-tag v-if="s.isWideTable" type="warning" size="small" effect="plain" class="wide-tag">Wide</el-tag>
                        </div>
                      </el-option>
                    </el-select>
                    <el-tooltip content="Different file formats are auto-detected as separate schema versions">
                      <el-icon class="ml-2 info-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </el-form-item>
                </el-col>
              </el-row>

              <!-- Single Schema Info Display (only one version) -->
              <el-row :gutter="20" v-else-if="hasAnySchema && selectedSchemaInfo">
                <el-col :span="24">
                  <el-form-item label="Schema">
                    <div class="single-schema-info">
                      <el-tag type="primary" size="default">v{{ selectedSchemaInfo.version }}</el-tag>
                      <span class="schema-meta">{{ formatNumber(selectedSchemaInfo.recordCount) }} records</span>
                      <el-tag v-if="selectedSchemaInfo.isWideTable" type="warning" size="small" effect="plain">Wide Table</el-tag>
                      <el-tooltip content="Schema is auto-detected from file formats. Multiple versions will appear if format changes.">
                        <el-icon class="ml-2 info-icon"><InfoFilled /></el-icon>
                      </el-tooltip>
                    </div>
                  </el-form-item>
                </el-col>
              </el-row>

              <!-- No Schema Warning -->
              <el-alert
                v-if="!hasAnySchema && !schemasLoading"
                type="warning"
                :closable="false"
                class="schema-warning-alert"
                show-icon
              >
                <template #title>No schema detected yet</template>
                <span>Schema will be auto-detected when metrics data is collected. Run a workflow to generate metrics first.</span>
              </el-alert>

              <!-- Schema Changes Alert -->
              <el-alert
                v-if="hasSchemaChanges"
                type="info"
                :closable="false"
                class="schema-alert"
              >
                <template #title>
                  Schema v{{ queryForm.schemaVersion }} has changes from v{{ previousSchemaVersion }}
                </template>
                <div class="schema-changes">
                  <span v-if="currentSchemaChanges?.addedDimensions?.length">
                    <el-tag type="success" size="small">+</el-tag>
                    Dimensions: {{ currentSchemaChanges.addedDimensions.join(', ') }}
                  </span>
                  <span v-if="currentSchemaChanges?.removedDimensions?.length">
                    <el-tag type="danger" size="small">-</el-tag>
                    Dimensions: {{ currentSchemaChanges.removedDimensions.join(', ') }}
                  </span>
                  <span v-if="currentSchemaChanges?.addedMetrics?.length">
                    <el-tag type="success" size="small">+</el-tag>
                    Metrics: {{ currentSchemaChanges.addedMetrics.join(', ') }}
                  </span>
                  <span v-if="currentSchemaChanges?.removedMetrics?.length">
                    <el-tag type="danger" size="small">-</el-tag>
                    Metrics: {{ currentSchemaChanges.removedMetrics.join(', ') }}
                  </span>
                </div>
              </el-alert>

              <el-row :gutter="20">
                <el-col :span="12">
                  <el-form-item label="Time Range">
                    <el-date-picker
                      v-model="queryForm.timeRange"
                      type="datetimerange"
                      range-separator="to"
                      start-placeholder="Start"
                      end-placeholder="End"
                      value-format="YYYY-MM-DDTHH:mm:ssZ"
                      size="default"
                      class="w-full"
                    />
                  </el-form-item>
                </el-col>
                <el-col :span="6">
                  <el-form-item label="Interval">
                    <el-select v-model="queryForm.interval" size="default" class="w-full">
                      <el-option label="1 Hour" value="1h" />
                      <el-option label="6 Hours" value="6h" />
                      <el-option label="1 Day" value="1d" />
                      <el-option label="1 Week" value="1w" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="6">
                  <el-form-item label=" ">
                    <el-button type="primary" size="default" :loading="querying" :icon="Search" @click="executeQuery">
                      Query
                    </el-button>
                  </el-form-item>
                </el-col>
              </el-row>

              <!-- Dimension Filters -->
              <el-form-item label="Dimensions" v-if="availableDimensions.length > 0">
                <div class="dimension-filters">
                  <div v-for="dim in availableDimensions" :key="dim" class="dimension-row">
                    <span class="dim-label">{{ dim }}:</span>
                    <el-select
                      v-model="queryForm.dimensions[dim]"
                      multiple
                      collapse-tags
                      collapse-tags-tooltip
                      :max-collapse-tags="2"
                      placeholder="All"
                      clearable
                      size="default"
                      class="dim-select"
                    >
                      <el-option
                        v-for="val in dimensionValues[dim] || []"
                        :key="val"
                        :label="val"
                        :value="val"
                      />
                    </el-select>
                  </div>
                </div>
              </el-form-item>

              <!-- Metric Selection -->
              <el-form-item label="Metrics" v-if="availableMetrics.length > 0">
                <el-checkbox-group v-model="queryForm.selectedMetrics">
                  <el-checkbox v-for="metric in availableMetrics" :key="metric" :label="metric">
                    {{ metric }}
                  </el-checkbox>
                </el-checkbox-group>
              </el-form-item>

              <!-- Chart Group Mode -->
              <el-form-item label="Chart Group">
                <el-radio-group v-model="queryForm.chartGroupMode" size="default" class="group-mode-radio">
                  <el-radio-button value="none">No Grouping</el-radio-button>
                  <el-radio-button value="dimension" :disabled="availableDimensions.length === 0">By Dimension</el-radio-button>
                  <el-radio-button value="metric" :disabled="queryForm.selectedMetrics.length <= 1">By Metric</el-radio-button>
                </el-radio-group>
              </el-form-item>

              <!-- Dimension Select (only when group by dimension) -->
              <el-form-item label="Group Dimension" v-if="queryForm.chartGroupMode === 'dimension' && availableDimensions.length > 0">
                <el-select
                  v-model="queryForm.chartGroupBy"
                  placeholder="Select dimension"
                  size="default"
                  class="group-by-select"
                >
                  <el-option
                    v-for="dim in availableDimensions"
                    :key="dim"
                    :label="dim"
                    :value="dim"
                  />
                </el-select>
                <span class="form-hint">Split into multiple charts by dimension value</span>
              </el-form-item>
            </el-form>
          </el-card>

          <!-- Results -->
          <template v-if="hasResults">
            <!-- Chart Controls -->
            <div class="chart-controls">
              <el-button-group size="small">
                <el-button 
                  :type="chartType === 'line' ? 'primary' : 'default'"
                  @click="chartType = 'line'"
                >
                  Line
                </el-button>
                <el-button 
                  :type="chartType === 'bar' ? 'primary' : 'default'"
                  @click="chartType = 'bar'"
                >
                  Bar
                </el-button>
              </el-button-group>
            </div>

            <!-- Single Chart (no grouping) -->
            <el-card v-if="queryForm.chartGroupMode === 'none'" class="chart-card glass-card">
              <template #header>
                <div class="card-header">
                  <span class="card-title">Trends</span>
                </div>
              </template>
              <div ref="chartRef" class="chart-container" v-loading="querying"></div>
            </el-card>

            <!-- Grouped Charts by Dimension -->
            <template v-else-if="queryForm.chartGroupMode === 'dimension'">
              <el-card 
                v-for="(chartGroup, idx) in groupedCharts" 
                :key="chartGroup.groupValue"
                class="chart-card grouped-chart glass-card"
              >
                <template #header>
                  <div class="card-header">
                    <span class="card-title">
                      <el-tag type="primary" effect="plain" size="small">{{ queryForm.chartGroupBy }}</el-tag>
                      {{ chartGroup.groupValue }}
                    </span>
                    <span class="chart-count">{{ chartGroup.groups.length }} series</span>
                  </div>
                </template>
                <div 
                  :ref="el => setGroupedChartRef(el, idx)" 
                  class="chart-container" 
                  v-loading="querying"
                ></div>
              </el-card>
            </template>

            <!-- Grouped Charts by Metric -->
            <template v-else-if="queryForm.chartGroupMode === 'metric'">
              <el-card 
                v-for="(metric, idx) in queryForm.selectedMetrics" 
                :key="metric"
                class="chart-card grouped-chart glass-card"
              >
                <template #header>
                  <div class="card-header">
                    <span class="card-title">
                      <el-tag type="success" effect="plain" size="small">Metric</el-tag>
                      {{ metric }}
                    </span>
                    <span class="chart-count">{{ dimensionGroups.length }} series</span>
                  </div>
                </template>
                <div 
                  :ref="el => setMetricChartRef(el, idx)" 
                  class="chart-container" 
                  v-loading="querying"
                ></div>
              </el-card>
            </template>

            <!-- Dimension Groups Table -->
            <el-card class="groups-card glass-card">
              <template #header>
                <div class="card-header">
                  <span class="card-title">Dimension Groups ({{ dimensionGroups.length }})</span>
                  <div class="card-actions">
                    <el-button size="small" @click="toggleAllSeries(true)">Show All</el-button>
                    <el-button size="small" @click="toggleAllSeries(false)">Hide All</el-button>
                  </div>
                </div>
              </template>

              <el-table :data="dimensionGroups" size="small" max-height="300" border>
                <el-table-column width="60" align="center">
                  <template #header>
                    <el-checkbox 
                      :model-value="allSeriesVisible" 
                      :indeterminate="someSeriesVisible"
                      @change="toggleAllSeries($event as boolean)"
                    />
                  </template>
                  <template #default="{ row }">
                    <el-checkbox 
                      :model-value="visibleSeries.has(row.key)" 
                      @change="toggleSeries(row.key)"
                    />
                  </template>
                </el-table-column>
                <el-table-column label="Color" width="60" align="center">
                  <template #default="{ row }">
                    <div 
                      class="color-dot" 
                      :style="{ backgroundColor: seriesColors[row.key] || '#999' }"
                    ></div>
                  </template>
                </el-table-column>
                <el-table-column 
                  v-for="dim in availableDimensions" 
                  :key="dim"
                  :prop="dim"
                  :label="dim"
                  min-width="150"
                  show-overflow-tooltip
                />
                <el-table-column 
                  v-for="metric in queryForm.selectedMetrics" 
                  :key="`stat-${metric}`"
                  :label="`${metric} (avg)`"
                  align="right"
                  min-width="100"
                >
                  <template #default="{ row }">
                    {{ formatNumber(row.stats?.[metric]?.avg) }}
                  </template>
                </el-table-column>
                <el-table-column prop="count" label="Count" width="80" align="right" />
              </el-table>
            </el-card>

            <!-- Raw Data Table (Collapsible) -->
            <el-card class="data-card glass-card">
              <template #header>
                <div class="card-header clickable" @click="showRawData = !showRawData">
                  <span class="card-title">
                    <el-icon class="collapse-icon" :class="{ expanded: showRawData }">
                      <ArrowRight />
                    </el-icon>
                    Raw Data ({{ totalRawRecords }} records)
                  </span>
                  <div class="card-actions" @click.stop>
                    <el-button :icon="Download" size="small" @click="exportCSV">Export CSV</el-button>
                  </div>
                </div>
              </template>

              <el-collapse-transition>
                <el-table v-show="showRawData" :data="rawResults" size="small" max-height="400" border stripe>
                  <el-table-column
                    v-for="dim in availableDimensions"
                    :key="`dim-${dim}`"
                    :label="dim"
                    min-width="120"
                  >
                    <template #default="{ row }">{{ row.dimensions?.[dim] || '-' }}</template>
                  </el-table-column>
                  <el-table-column
                    v-for="metric in availableMetrics"
                    :key="`metric-${metric}`"
                    :label="metric"
                    align="right"
                    min-width="100"
                  >
                    <template #default="{ row }">{{ formatNumber(row.metrics?.[metric]) }}</template>
                  </el-table-column>
                  <el-table-column prop="sourceFile" label="Source File" min-width="200" show-overflow-tooltip />
                  <el-table-column label="Collected At" width="160">
                    <template #default="{ row }">{{ formatDate(row.collectedAt) }}</template>
                  </el-table-column>
                </el-table>
              </el-collapse-transition>
            </el-card>
          </template>

          <!-- Empty State -->
          <el-empty v-else-if="!querying" description="Run a query to see metrics data" />
          </div>
        </div>
      </el-tab-pane>

      <!-- Settings Tab (only with config) -->
      <el-tab-pane v-if="config" label="Settings" name="settings">
        <template #label>
          <span class="tab-label">
            <el-icon><Setting /></el-icon>
            Settings
          </span>
        </template>

        <div class="settings-content" v-if="config">
          <!-- Basic Info -->
          <el-card class="settings-card glass-card">
            <template #header>
              <div class="card-header">
                <span class="card-title">Basic Information</span>
                <div class="card-actions" v-if="!isEditingBasicInfo">
                  <el-button type="primary" size="small" :icon="Edit" @click="startEditBasicInfo">Edit</el-button>
                </div>
              </div>
            </template>

            <!-- View Mode -->
            <el-descriptions v-if="!isEditingBasicInfo" :column="2" border>
              <el-descriptions-item label="Name">{{ config.name }}</el-descriptions-item>
              <el-descriptions-item label="Enabled">
                <el-tag :type="config.enabled ? 'success' : 'info'" size="small">
                  {{ config.enabled ? 'Enabled' : 'Disabled' }}
                </el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="Description" :span="2">
                {{ config.description || '-' }}
              </el-descriptions-item>
              <el-descriptions-item label="GitHub Owner">{{ config.githubOwner }}</el-descriptions-item>
              <el-descriptions-item label="GitHub Repo">{{ config.githubRepo }}</el-descriptions-item>
              <el-descriptions-item label="Runner Set Namespace">{{ config.runnerSetNamespace }}</el-descriptions-item>
              <el-descriptions-item label="Runner Set Name">{{ config.runnerSetName }}</el-descriptions-item>
              <el-descriptions-item label="Workflow Filter">{{ config.workflowFilter || '-' }}</el-descriptions-item>
              <el-descriptions-item label="Branch Filter">{{ config.branchFilter || '-' }}</el-descriptions-item>
              <el-descriptions-item label="File Patterns" :span="2">
                <el-tag v-for="pattern in config.decodedFilePatterns" :key="pattern" size="small" class="pattern-tag">
                  {{ pattern }}
                </el-tag>
                <span v-if="!config.decodedFilePatterns?.length" class="text-muted">-</span>
              </el-descriptions-item>
              <el-descriptions-item label="Created At">{{ formatDate(config.createdAt) }}</el-descriptions-item>
              <el-descriptions-item label="Last Checked">{{ config.lastCheckedAt ? formatDate(config.lastCheckedAt) : 'Never' }}</el-descriptions-item>
            </el-descriptions>

            <!-- Edit Mode -->
            <el-form 
              v-else 
              :model="basicInfoForm" 
              label-width="160px" 
              label-position="right"
              class="basic-info-form"
            >
              <el-row :gutter="20">
                <el-col :span="12">
                  <el-form-item label="Name" required>
                    <el-input v-model="basicInfoForm.name" placeholder="Config name" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="Enabled">
                    <el-switch v-model="basicInfoForm.enabled" />
                  </el-form-item>
                </el-col>
              </el-row>
              
              <el-form-item label="Description">
                <el-input 
                  v-model="basicInfoForm.description" 
                  type="textarea" 
                  :rows="2"
                  placeholder="Optional description"
                />
              </el-form-item>
              
              <el-row :gutter="20">
                <el-col :span="12">
                  <el-form-item label="GitHub Owner" required>
                    <el-input v-model="basicInfoForm.githubOwner" placeholder="e.g. AMD-AGI" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="GitHub Repo" required>
                    <el-input v-model="basicInfoForm.githubRepo" placeholder="e.g. my-repo" />
                  </el-form-item>
                </el-col>
              </el-row>
              
              <el-row :gutter="20">
                <el-col :span="12">
                  <el-form-item label="Runner Set Namespace" required>
                    <el-input v-model="basicInfoForm.runnerSetNamespace" placeholder="e.g. github-runners" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="Runner Set Name" required>
                    <el-input v-model="basicInfoForm.runnerSetName" placeholder="e.g. my-runner-set" />
                  </el-form-item>
                </el-col>
              </el-row>
              
              <el-row :gutter="20">
                <el-col :span="12">
                  <el-form-item label="Workflow Filter">
                    <el-input 
                      v-model="basicInfoForm.workflowFilter" 
                      placeholder="e.g. nightly-build.yml"
                    />
                    <span class="form-hint">Filter by workflow file name (optional)</span>
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="Branch Filter">
                    <el-input 
                      v-model="basicInfoForm.branchFilter" 
                      placeholder="e.g. main, develop"
                    />
                    <span class="form-hint">Filter by branch name (optional)</span>
                  </el-form-item>
                </el-col>
              </el-row>
              
              <el-form-item label="File Patterns">
                <div class="file-patterns-editor">
                  <div 
                    v-for="(pattern, index) in basicInfoForm.filePatterns" 
                    :key="index"
                    class="pattern-item"
                  >
                    <el-input 
                      v-model="basicInfoForm.filePatterns[index]" 
                      placeholder="e.g. **/results/*.csv"
                      class="pattern-input"
                    />
                    <el-button 
                      type="danger" 
                      :icon="Delete" 
                      circle 
                      size="small"
                      @click="removeFilePattern(index)"
                      :disabled="basicInfoForm.filePatterns.length <= 1"
                    />
                  </div>
                  <el-button 
                    type="primary" 
                    :icon="Plus" 
                    size="small" 
                    @click="addFilePattern"
                    class="add-pattern-btn"
                  >
                    Add Pattern
                  </el-button>
                  <div class="form-hint">
                    Glob patterns for metrics files. Examples: **/metrics.json, **/results/*.csv
                  </div>
                </div>
              </el-form-item>
              
              <el-form-item>
                <el-button type="primary" :loading="savingBasicInfo" @click="saveBasicInfo">
                  Save Changes
                </el-button>
                <el-button @click="cancelEditBasicInfo">Cancel</el-button>
              </el-form-item>
            </el-form>
          </el-card>

          <!-- Schema Versions -->
          <el-card class="settings-card glass-card">
            <template #header>
              <div class="card-header">
                <span class="card-title">Schema Versions</span>
                <span class="card-hint">Auto-detected from file formats</span>
              </div>
            </template>
            
            <el-table 
              :data="schemaVersions" 
              size="small" 
              v-loading="schemasLoading"
              stripe
            >
              <el-table-column prop="version" label="Version" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.isActive ? 'primary' : 'info'" size="small">
                    v{{ row.version }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="Type" width="100" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.isWideTable" type="warning" size="small" effect="plain">Wide</el-tag>
                  <el-tag v-else type="info" size="small" effect="plain">Normal</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="Dimensions" min-width="180">
                <template #default="{ row }">
                  <div class="field-tags">
                    <el-tag 
                      v-for="f in (row.dimensionFields || []).slice(0, 5)" 
                      :key="f" 
                      size="small" 
                      type="info"
                      effect="plain"
                      class="field-tag"
                    >
                      {{ f }}
                    </el-tag>
                    <el-tag 
                      v-if="(row.dimensionFields || []).length > 5" 
                      size="small" 
                      type="info"
                      effect="plain"
                    >
                      +{{ row.dimensionFields.length - 5 }}
                    </el-tag>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="Metrics" min-width="180">
                <template #default="{ row }">
                  <div class="field-tags">
                    <el-tag 
                      v-for="f in (row.metricFields || []).slice(0, 5)" 
                      :key="f" 
                      size="small" 
                      type="success"
                      effect="plain"
                      class="field-tag"
                    >
                      {{ f }}
                    </el-tag>
                    <el-tag 
                      v-if="(row.metricFields || []).length > 5" 
                      size="small" 
                      type="success"
                      effect="plain"
                    >
                      +{{ row.metricFields.length - 5 }}
                    </el-tag>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="recordCount" label="Records" width="100" align="right">
                <template #default="{ row }">
                  {{ formatNumber(row.recordCount) }}
                </template>
              </el-table-column>
              <el-table-column label="Time Range" width="200">
                <template #default="{ row }">
                  <span v-if="row.firstSeenAt">
                    {{ formatDate(row.firstSeenAt) }}
                    <span class="text-muted"> - </span>
                    {{ formatDate(row.lastSeenAt) }}
                  </span>
                  <span v-else class="text-muted">-</span>
                </template>
              </el-table-column>
              <el-table-column label="Status" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.isActive ? 'success' : 'info'" effect="plain" size="small">
                    {{ row.isActive ? 'Active' : 'Historical' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="Source" width="80" align="center">
                <template #default="{ row }">
                  <el-tag type="info" effect="plain" size="small">
                    {{ row.generatedBy || 'ai' }}
                  </el-tag>
                </template>
              </el-table-column>
            </el-table>
            
            <el-empty v-if="schemaVersions.length === 0 && !schemasLoading" description="No schema versions detected yet" />
          </el-card>

          <!-- Display Settings -->
          <el-card class="settings-card glass-card">
            <template #header>
              <div class="card-header">
                <span class="card-title">Display Settings</span>
                <span class="card-hint">Configure default display options for the Explorer tab</span>
              </div>
            </template>
            <el-form label-width="200px" label-position="right">
              <el-form-item label="Default Chart Group">
                <el-radio-group v-model="displaySettingsForm.defaultChartGroupMode" size="default">
                  <el-radio-button value="none">No Grouping</el-radio-button>
                  <el-radio-button value="dimension">By Dimension</el-radio-button>
                  <el-radio-button value="metric">By Metric</el-radio-button>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="Group Dimension" v-if="displaySettingsForm.defaultChartGroupMode === 'dimension'">
                <el-select
                  v-model="displaySettingsForm.defaultChartGroupBy"
                  placeholder="Select dimension"
                  size="default"
                  class="display-select mr-2"
                >
                  <el-option
                    v-for="dim in availableDimensions"
                    :key="dim"
                    :label="dim"
                    :value="dim"
                  />
                </el-select>
                <span class="form-hint">Split charts by this dimension</span>
              </el-form-item>
              <el-form-item label="Default Chart Type">
                <el-radio-group v-model="displaySettingsForm.defaultChartType" size="default">
                  <el-radio value="line">Line</el-radio>
                  <el-radio value="bar">Bar</el-radio>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="Show Raw Data by Default">
                <el-switch v-model="displaySettingsForm.showRawDataByDefault" size="default" class="mr-2" />
                <span class="form-hint">Expand raw data table by default</span>
              </el-form-item>
              <el-form-item>
                <el-button type="primary" size="default" :loading="savingDisplaySettings" @click="saveDisplaySettings">
                  Save Display Settings
                </el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- Backfill Dialog -->
    <el-dialog 
      v-model="showBackfillDialog" 
      title="Trigger Backfill"
      width="520px" 
      :close-on-click-modal="false"
      destroy-on-close
      class="backfill-dialog"
    >
      <el-form :model="backfillForm" label-width="120px">
        <el-form-item label="Time Range">
          <el-date-picker
            v-model="backfillForm.timeRange"
            type="datetimerange"
            range-separator="to"
            start-placeholder="Start"
            end-placeholder="End"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            size="default"
            class="w-full"
          />
        </el-form-item>
        <el-form-item label="Dry Run">
          <el-switch v-model="backfillForm.dryRun" size="default" />
          <span style="margin-left: 12px; font-size: 12px; color: var(--el-text-color-secondary);">Preview only, don't actually process</span>
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button size="default" @click="showBackfillDialog = false">Cancel</el-button>
        <el-button type="primary" size="default" :loading="backfillLoading" @click="triggerBackfillAction">
          Start Backfill
        </el-button>
      </template>
    </el-dialog>

    <!-- Create Config Dialog (Runner Set Centric) -->
    <el-dialog
      v-model="showCreateConfigDialog"
      title="Create Metrics Collection Config"
      width="600px"
      :close-on-click-modal="false"
      destroy-on-close
      @closed="resetCreateConfigForm"
    >
      <el-form :model="createConfigForm" label-width="140px" label-position="right">
        <el-form-item label="Config Name" required>
          <el-input v-model="createConfigForm.name" placeholder="e.g. MI325 Benchmark Metrics" />
        </el-form-item>

        <el-form-item label="Description">
          <el-input
            v-model="createConfigForm.description"
            type="textarea"
            :rows="2"
            placeholder="Optional description"
          />
        </el-form-item>

        <el-divider content-position="left">File Patterns</el-divider>

        <el-form-item label="Patterns" required>
          <div class="patterns-editor">
            <div v-for="(pattern, index) in createConfigForm.filePatterns" :key="index" class="pattern-row">
              <el-input v-model="createConfigForm.filePatterns[index]" placeholder="e.g. **/summary.csv" />
              <el-button :icon="Delete" circle @click="removeConfigPattern(index)" :disabled="createConfigForm.filePatterns.length <= 1" />
            </div>
            <el-button :icon="Plus" @click="addConfigPattern" class="add-pattern-btn">
              Add Pattern
            </el-button>
          </div>
        </el-form-item>

        <el-divider content-position="left">Filters (Optional)</el-divider>

        <el-form-item label="Workflow Filter">
          <el-input v-model="createConfigForm.workflowFilter" placeholder="e.g. benchmark.yml" />
          <span class="form-hint">Filter by workflow file name</span>
        </el-form-item>

        <el-form-item label="Branch Filter">
          <el-input v-model="createConfigForm.branchFilter" placeholder="e.g. main" />
          <span class="form-hint">Filter by branch name</span>
        </el-form-item>

        <el-form-item label="Enabled">
          <el-switch v-model="createConfigForm.enabled" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showCreateConfigDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="creatingConfig" @click="submitCreateConfig">
          Create Config
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft, Link, Box, List, DataAnalysis, Setting, Search,
  Plus, Download, VideoPlay, Document, InfoFilled, View, ArrowRight,
  Edit, Delete, TrendCharts
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import dayjs from 'dayjs'
import {
  getConfig,
  getRunsByConfig,
  getRunsByRunnerSet,
  getRunnerSetById,
  getRunsByRunnerSetId,
  getConfigByRunnerSetId,
  getStatsByRunnerSetId,
  createConfigForRunnerSet,
  triggerBackfillByRunnerSetId,
  updateConfig as updateConfigApi,
  getDimensions,
  getMetricFields,
  getMetricsTrends,
  queryMetrics,
  triggerBackfill,
  getSchemasByConfig,
  getSchemaChanges,
  type RunnerSet,
  type RunnerSetStats,
  type WorkflowConfig,
  type WorkflowRun,
  type MetricRecord,
  type WorkflowSchema,
  type SchemaChange
} from '@/services/workflow-metrics'
import { useClusterSync } from '@/composables/useClusterSync'

// Chart colors palette
const COLORS = [
  '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
  '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc', '#48b8d0'
]

const route = useRoute()
const router = useRouter()
const { selectedCluster, urlCluster, navigateWithCluster } = useClusterSync()

// Determine the mode based on route
const isRunnerSetCentric = computed(() => 
  route.meta?.runnerSetCentric === true || route.path.includes('/runner-sets/')
)
const routeId = computed(() => Number(route.params.id))

// Legacy mode check (for backward compatibility)
const isLegacyRunnerSetMode = computed(() => 
  !isRunnerSetCentric.value && routeId.value === 0
)

// Config ID (only valid for config-centric routes)
const configId = computed(() => isRunnerSetCentric.value ? 0 : routeId.value)

// Runner Set ID (for runner-set-centric routes)
const runnerSetId = computed(() => isRunnerSetCentric.value ? routeId.value : 0)

// Effective config ID - uses loaded config.id in runner-set-centric mode
// This is needed because in runner-set-centric mode, we first fetch the config
// and then need to use its ID for schema/metrics APIs
const effectiveConfigId = computed(() => {
  if (isRunnerSetCentric.value) {
    return config.value?.id || 0
  }
  return configId.value
})

// Check if we're in a mode without config (runner-set-centric or legacy with id=0)
const isRunnerSetMode = computed(() => isRunnerSetCentric.value || isLegacyRunnerSetMode.value)

// Runner set info (fetched from API for runner-set-centric, or from query for legacy)
const runnerSet = ref<RunnerSet | null>(null)
const runnerSetStats = ref<RunnerSetStats | null>(null)

const runnerSetInfo = computed(() => {
  if (runnerSet.value) {
    return {
      id: runnerSet.value.id,
      namespace: runnerSet.value.namespace,
      name: runnerSet.value.name,
      githubOwner: runnerSet.value.githubOwner || '',
      githubRepo: runnerSet.value.githubRepo || ''
    }
  }
  // Fallback for legacy mode
  return {
    id: 0,
    namespace: route.query.runnerSetNamespace as string || '',
    name: route.query.runnerSetName as string || '',
    githubOwner: route.query.githubOwner as string || '',
    githubRepo: route.query.githubRepo as string || ''
  }
})

// State
const config = ref<WorkflowConfig | null>(null)
const activeTab = ref('runs')
const analyticsView = ref<'metrics'>('metrics')

// Runs state
const runs = ref<WorkflowRun[]>([])
const runsLoading = ref(false)
const runsFilter = reactive({
  status: undefined as string | undefined,
  triggerSource: undefined as string | undefined
})
const runsPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})
const runStats = reactive({
  pending: 0,
  completed: 0,
  failed: 0
})

// Running runs for banner
const runningRuns = computed(() => 
  runs.value.filter(r => {
    // Check new workflow status first
    if (r.workflowStatus) {
      return r.workflowStatus === 'in_progress' || r.workflowStatus === 'queued'
    }
    // Fallback to legacy status
    return r.status === 'pending' || r.status === 'collecting' || 
           r.status === 'workload_running' || r.status === 'workload_pending'
  })
)

// Table filters - Workflow Status (from GitHub)
const workflowStatusFilters = [
  { text: 'Queued', value: 'queued' },
  { text: 'In Progress', value: 'in_progress' },
  { text: 'Completed', value: 'completed' },
  { text: 'Waiting', value: 'waiting' }
]

// Table filters - Collection Status (internal)
const collectionStatusFilters = [
  { text: 'Pending', value: 'pending' },
  { text: 'Collecting', value: 'collecting' },
  { text: 'Completed', value: 'completed' },
  { text: 'Failed', value: 'failed' },
  { text: 'Skipped', value: 'skipped' }
]

// Legacy status filters (for backward compatibility)
const statusFilters = [
  { text: 'Pending', value: 'pending' },
  { text: 'Collecting', value: 'collecting' },
  { text: 'Completed', value: 'completed' },
  { text: 'Failed', value: 'failed' }
]

const triggerFilters = [
  { text: 'Realtime', value: 'realtime' },
  { text: 'Backfill', value: 'backfill' },
  { text: 'Manual', value: 'manual' }
]

const filteredStatus = computed(() => runsFilter.status ? [runsFilter.status] : [])
const filteredTrigger = computed(() => runsFilter.triggerSource ? [runsFilter.triggerSource] : [])

// Schema versions state
const schemaVersions = ref<WorkflowSchema[]>([])
const schemaChanges = ref<SchemaChange[]>([])
const currentSchemaVersion = ref<number | null>(null)
const schemasLoading = ref(false)

// Explorer state
const availableDimensions = ref<string[]>([])
const availableMetrics = ref<string[]>([])
const dimensionValues = ref<Record<string, string[]>>({})
const querying = ref(false)
const queryForm = reactive({
  timeRange: [] as string[],
  schemaVersion: null as number | null,
  dimensions: {} as Record<string, string[]>,
  selectedMetrics: [] as string[],
  interval: '1d',
  limit: 100,
  chartGroupMode: 'none' as 'none' | 'dimension' | 'metric',
  chartGroupBy: '' as string
})

// Results
const rawResults = ref<MetricRecord[]>([])
const totalRawRecords = ref(0)

interface DimensionGroup {
  key: string
  dimensions: Record<string, string>
  stats: Record<string, { avg: number; sum: number; min: number; max: number }>
  count: number
  data: Record<string, { timestamp: string; value: number }[]>
  [key: string]: any
}

const dimensionGroups = ref<DimensionGroup[]>([])
const visibleSeries = ref<Set<string>>(new Set())
const seriesColors = ref<Record<string, string>>({})
const chartType = ref<'line' | 'bar'>('line')
const showRawData = ref(false)

const hasResults = computed(() => 
  rawResults.value.length > 0 || dimensionGroups.value.length > 0
)

// Schema version related computed
const hasMultipleSchemas = computed(() => schemaVersions.value.length > 1)
const hasAnySchema = computed(() => schemaVersions.value.length > 0)

const selectedSchemaInfo = computed(() => {
  // If specific version selected, return that
  if (queryForm.schemaVersion) {
    return schemaVersions.value.find(s => s.version === queryForm.schemaVersion)
  }
  // Otherwise return the first (latest) schema if exists
  return schemaVersions.value.length > 0 ? schemaVersions.value[0] : null
})

// Get the schema ID for API calls (maps version to id)
const selectedSchemaId = computed(() => {
  return selectedSchemaInfo.value?.id || null
})

const previousSchemaVersion = computed(() => {
  if (!queryForm.schemaVersion || schemaVersions.value.length < 2) return null
  const currentIdx = schemaVersions.value.findIndex(s => s.version === queryForm.schemaVersion)
  if (currentIdx < 0 || currentIdx >= schemaVersions.value.length - 1) return null
  return schemaVersions.value[currentIdx + 1]?.version
})

const currentSchemaChanges = computed(() => {
  if (!queryForm.schemaVersion || !previousSchemaVersion.value) return null
  return schemaChanges.value.find(
    c => c.toVersion === queryForm.schemaVersion && c.fromVersion === previousSchemaVersion.value
  )
})

const hasSchemaChanges = computed(() => {
  return currentSchemaChanges.value && (
    currentSchemaChanges.value.addedDimensions.length > 0 ||
    currentSchemaChanges.value.removedDimensions.length > 0 ||
    currentSchemaChanges.value.addedMetrics.length > 0 ||
    currentSchemaChanges.value.removedMetrics.length > 0
  )
})

// Check if we have GitHub info for generating links
const hasGithubInfo = computed(() => {
  if (config.value) {
    return !!(config.value.githubOwner && config.value.githubRepo)
  }
  return !!(runnerSetInfo.value.githubOwner && runnerSetInfo.value.githubRepo)
})

// Grouped charts data when chartGroupBy is selected
const groupedCharts = computed(() => {
  if (!queryForm.chartGroupBy || dimensionGroups.value.length === 0) {
    return []
  }
  
  const groupKey = queryForm.chartGroupBy
  const grouped = new Map<string, DimensionGroup[]>()
  
  for (const group of dimensionGroups.value) {
    const groupValue = group.dimensions[groupKey] || 'Unknown'
    if (!grouped.has(groupValue)) {
      grouped.set(groupValue, [])
    }
    grouped.get(groupValue)!.push(group)
  }
  
  return Array.from(grouped.entries()).map(([value, groups]) => ({
    groupValue: value,
    groups
  })).sort((a, b) => a.groupValue.localeCompare(b.groupValue))
})

const allSeriesVisible = computed(() => 
  dimensionGroups.value.length > 0 && 
  dimensionGroups.value.every(g => visibleSeries.value.has(g.key))
)

const someSeriesVisible = computed(() => 
  !allSeriesVisible.value && 
  dimensionGroups.value.some(g => visibleSeries.value.has(g.key))
)

// Chart
const chartRef = ref<HTMLElement>()
let chartInstance: echarts.ECharts | null = null

// Grouped charts by dimension
const groupedChartRefs = ref<(HTMLElement | null)[]>([])
const groupedChartInstances: echarts.ECharts[] = []

const setGroupedChartRef = (el: any, index: number) => {
  groupedChartRefs.value[index] = el as HTMLElement | null
}

// Grouped charts by metric
const metricChartRefs = ref<(HTMLElement | null)[]>([])
const metricChartInstances: echarts.ECharts[] = []

const setMetricChartRef = (el: any, index: number) => {
  metricChartRefs.value[index] = el as HTMLElement | null
}

// Resize handler
const handleResize = () => {
  chartInstance?.resize()
  groupedChartInstances.forEach(instance => instance.resize())
  metricChartInstances.forEach(instance => instance.resize())
}

// Backfill
const showBackfillDialog = ref(false)
const backfillLoading = ref(false)
const backfillForm = reactive({
  timeRange: [] as string[],
  dryRun: false
})

// Create Config Dialog (for runner-set-centric mode)
const showCreateConfigDialog = ref(false)
const creatingConfig = ref(false)
const createConfigForm = reactive({
  name: '',
  description: '',
  filePatterns: [''] as string[],
  workflowFilter: '',
  branchFilter: '',
  enabled: true
})

// Display Settings Form
const displaySettingsForm = reactive({
  defaultChartGroupMode: 'none' as 'none' | 'dimension' | 'metric',
  defaultChartGroupBy: '' as string,
  defaultChartType: 'line' as 'line' | 'bar',
  showRawDataByDefault: false
})
const savingDisplaySettings = ref(false)

// Basic Info Edit Form
const isEditingBasicInfo = ref(false)
const savingBasicInfo = ref(false)
const basicInfoForm = reactive({
  name: '',
  description: '',
  githubOwner: '',
  githubRepo: '',
  runnerSetNamespace: '',
  runnerSetName: '',
  workflowFilter: '',
  branchFilter: '',
  filePatterns: [''] as string[],
  enabled: true
})

// Initialize
onMounted(async () => {
  if (isRunnerSetCentric.value && runnerSetId.value) {
    // Runner-set-centric mode: fetch runner set first
    await fetchRunnerSet()
    // Try to fetch associated config (may return null)
    await fetchConfigByRunnerSet()
    // Load schema and metadata if config exists
    if (config.value) {
      await loadSchemaVersions()
      await loadMetadata(selectedSchemaId.value ?? undefined)
    }
  } else if (!isRunnerSetMode.value) {
    // Config-centric mode (legacy)
    await fetchConfig()
    await loadSchemaVersions()
    await loadMetadata(selectedSchemaId.value ?? undefined)
  }
  await fetchRuns()

  // Set default time range
  const now = dayjs()
  queryForm.timeRange = [
    now.subtract(30, 'day').format('YYYY-MM-DDTHH:mm:ssZ'),
    now.format('YYYY-MM-DDTHH:mm:ssZ')
  ]
  backfillForm.timeRange = [
    now.subtract(7, 'day').format('YYYY-MM-DDTHH:mm:ssZ'),
    now.format('YYYY-MM-DDTHH:mm:ssZ')
  ]

  window.addEventListener('resize', handleResize)
  
  // Start observing theme changes
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class']
  })
})

// Watch for cluster changes and reload data
watch(() => selectedCluster.value, async (newCluster, oldCluster) => {
  if (newCluster && newCluster !== oldCluster) {
    // Reset and reload data when cluster changes
    runs.value = []
    rawResults.value = []
    schemaVersions.value = []
    schemaChanges.value = []
    queryForm.schemaVersion = null
    runnerSet.value = null
    runnerSetStats.value = null
    
    // Dispose chart instance if it exists
    if (chartInstance) {
      chartInstance.dispose()
      chartInstance = null
    }
    
    if (isRunnerSetCentric.value && runnerSetId.value) {
      await fetchRunnerSet()
      await fetchConfigByRunnerSet()
      if (config.value) {
        await loadSchemaVersions()
        await loadMetadata(selectedSchemaId.value ?? undefined)
      }
    } else if (!isRunnerSetMode.value) {
      await fetchConfig()
      await loadSchemaVersions()
      await loadMetadata(selectedSchemaId.value ?? undefined)
    }
    await fetchRuns()
  }
})

// Watch for schema version changes and reload metadata
watch(() => queryForm.schemaVersion, async (newVersion, oldVersion) => {
  // Reload metadata when schema version changes, as long as we have a valid config
  if (newVersion && newVersion !== oldVersion && effectiveConfigId.value) {
    // Reset dimension filters and selected metrics when schema changes
    queryForm.dimensions = {}
    queryForm.selectedMetrics = []
    
    // Find the schema for the new version
    const schemaForVersion = schemaVersions.value.find(s => s.version === newVersion)
    
    // Use schema's own dimension/metric fields instead of relying on API
    // This ensures we use the exact fields defined in the schema
    if (schemaForVersion) {
      availableDimensions.value = schemaForVersion.dimensionFields || []
      availableMetrics.value = schemaForVersion.metricFields || []
      
      // Set default metrics based on chart group mode
      if (availableMetrics.value.length > 0) {
        if (queryForm.chartGroupMode === 'metric') {
          queryForm.selectedMetrics = [...availableMetrics.value]
        } else {
          queryForm.selectedMetrics = [availableMetrics.value[0]]
        }
      }
    }
    
    // Load dimension values from API (for filter dropdowns)
    await loadDimensionValues(schemaForVersion?.id ?? undefined)
    
    // Reset chartGroupBy if it's not in the new schema's dimensions
    if (queryForm.chartGroupBy && !availableDimensions.value.includes(queryForm.chartGroupBy)) {
      queryForm.chartGroupBy = ''
    }
  }
})

// Watch chart changes
watch([chartType, visibleSeries], () => {
  if (hasResults.value) {
    renderAllCharts()
  }
}, { deep: true })

// Watch theme changes and re-render charts
const themeObserver = new MutationObserver(() => {
  if (hasResults.value) {
    renderAllCharts()
  }
})

onBeforeUnmount(() => {
  // Stop observing theme changes
  themeObserver.disconnect()
  
  // Dispose chart instances
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
  
  groupedChartInstances.forEach(instance => instance.dispose())
  groupedChartInstances.length = 0
  
  metricChartInstances.forEach(instance => instance.dispose())
  metricChartInstances.length = 0
  
  // Remove resize listener
  window.removeEventListener('resize', handleResize)
})

// Watch chart group mode and group by changes
watch([() => queryForm.chartGroupMode, () => queryForm.chartGroupBy], async () => {
  if (hasResults.value) {
    await nextTick()
    renderAllCharts()
  }
})

// Auto-select all metrics when switching to metric group mode
watch(() => queryForm.chartGroupMode, (newMode) => {
  if (newMode === 'metric' && availableMetrics.value.length > 0) {
    queryForm.selectedMetrics = [...availableMetrics.value]
  }
})

// Auto-query when switching to analytics tab with display settings
const hasAutoQueried = ref(false)
watch(() => activeTab.value, async (newTab) => {
  if (newTab === 'analytics' && !hasAutoQueried.value && !hasResults.value) {
    const settings = config.value?.displaySettings as any
    // Check if we have display settings configured (support both snake_case and camelCase)
    const hasDisplaySettings = settings && (
      settings?.default_chart_group_mode || settings?.defaultChartGroupMode ||
      settings?.default_chart_group_by || settings?.defaultChartGroupBy ||
      settings?.default_chart_type || settings?.defaultChartType
    )
    
    // Auto-query if there are display settings and at least one metric selected
    if (hasDisplaySettings && queryForm.selectedMetrics.length > 0) {
      hasAutoQueried.value = true
      await nextTick()
      executeQuery()
    }
  }
})

// Methods
const goBack = () => {
  navigateWithCluster('/github-workflow')
}

// Navigate to run detail page
const goToRunDetail = (run: WorkflowRun) => {
  navigateWithCluster(`/github-workflow/runs/${run.id}`)
}

// Handle table row click
const handleRowClick = (row: WorkflowRun, column: any) => {
  // Don't navigate if clicking on actions column or links
  if (column?.property === 'actions') return
  goToRunDetail(row)
}

// Get row class name for styling
const getRowClassName = ({ row }: { row: WorkflowRun }) => {
  const classes = ['clickable-row']
  // Check new workflow status first
  if (row.workflowStatus) {
    if (row.workflowStatus === 'in_progress' || row.workflowStatus === 'queued') {
      classes.push('running-row')
    }
  } else if (row.status === 'pending' || row.status === 'collecting' || 
      row.status === 'workload_running' || row.status === 'workload_pending') {
    classes.push('running-row')
  }
  return classes.join(' ')
}

// Decode base64 encoded filePatterns
const decodeFilePatterns = (encoded: string): string[] => {
  if (!encoded) return []
  try {
    const decoded = atob(encoded)
    return JSON.parse(decoded)
  } catch {
    return Array.isArray(encoded) ? encoded : [encoded]
  }
}

// Decode base64 encoded displaySettings
const decodeDisplaySettings = (encoded: string | object | undefined): Record<string, any> => {
  if (!encoded) return {}
  if (typeof encoded === 'object') return encoded
  try {
    const decoded = atob(encoded)
    return JSON.parse(decoded) || {}
  } catch {
    return {}
  }
}

// Decode base64 encoded JSON array field (for schema fields)
const decodeJsonArrayField = (encoded: string | string[] | undefined): string[] => {
  if (!encoded) return []
  // Already decoded array
  if (Array.isArray(encoded)) return encoded
  // Try to decode base64
  try {
    const decoded = atob(encoded)
    const parsed = JSON.parse(decoded)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    // If decode fails, maybe it's just a plain string or already decoded
    try {
      const parsed = JSON.parse(encoded)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
}

const fetchConfig = async () => {
  try {
    const res = await getConfig(configId.value)
    // Decode filePatterns and displaySettings from base64
    config.value = {
      ...res,
      decodedFilePatterns: decodeFilePatterns(res.filePatterns as unknown as string),
      displaySettings: decodeDisplaySettings(res.displaySettings as unknown as string)
    }
    
    // Apply display settings from config
    applyDisplaySettings()
  } catch (error) {
    console.error('Failed to fetch config:', error)
    ElMessage.error('Failed to load configuration')
  }
}

// Fetch runner set for runner-set-centric mode
const fetchRunnerSet = async () => {
  if (!runnerSetId.value) return
  try {
    runnerSet.value = await getRunnerSetById(runnerSetId.value)
    runnerSetStats.value = await getStatsByRunnerSetId(runnerSetId.value)
  } catch (error) {
    console.error('Failed to fetch runner set:', error)
    ElMessage.error('Failed to load runner set')
  }
}

// Fetch config associated with runner set (may return null)
const fetchConfigByRunnerSet = async () => {
  if (!runnerSetId.value) return
  try {
    const res = await getConfigByRunnerSetId(runnerSetId.value)
    if (res) {
      config.value = {
        ...res,
        decodedFilePatterns: decodeFilePatterns(res.filePatterns as unknown as string),
        displaySettings: decodeDisplaySettings(res.displaySettings as unknown as string)
      }
      applyDisplaySettings()
    } else {
      config.value = null
    }
  } catch (error) {
    console.error('Failed to fetch config by runner set:', error)
    // Config not found is OK - runner set can exist without config
    config.value = null
  }
}

const applyDisplaySettings = () => {
  const settings = config.value?.displaySettings as any
  if (!settings) return
  
  // Support both snake_case (from backend) and camelCase
  const defaultGroupMode = settings?.default_chart_group_mode || settings?.defaultChartGroupMode || 'none'
  const defaultGroupBy = settings?.default_chart_group_by || settings?.defaultChartGroupBy || ''
  const defaultType = settings?.default_chart_type || settings?.defaultChartType || 'line'
  const showRawDataDefault = settings?.show_raw_data_by_default ?? settings?.showRawDataByDefault ?? false
  
  // Populate the form with current settings
  displaySettingsForm.defaultChartGroupMode = defaultGroupMode as 'none' | 'dimension' | 'metric'
  displaySettingsForm.defaultChartGroupBy = defaultGroupBy
  displaySettingsForm.defaultChartType = defaultType as 'line' | 'bar'
  displaySettingsForm.showRawDataByDefault = showRawDataDefault
  
  // Apply settings to the explorer view
  queryForm.chartGroupMode = defaultGroupMode as 'none' | 'dimension' | 'metric'
  if (defaultGroupMode === 'dimension' && defaultGroupBy) {
    queryForm.chartGroupBy = defaultGroupBy
  }
  
  // For metric group mode, select all metrics
  if (defaultGroupMode === 'metric' && availableMetrics.value.length > 0) {
    queryForm.selectedMetrics = [...availableMetrics.value]
  }
  
  if (defaultType) {
    chartType.value = defaultType as 'line' | 'bar'
  }
  
  showRawData.value = showRawDataDefault
}

const saveDisplaySettings = async () => {
  if (!config.value) return
  
  savingDisplaySettings.value = true
  try {
    // Backend requires all fields, so send complete config with updated displaySettings
    await updateConfigApi(config.value.id, {
      name: config.value.name,
      description: config.value.description,
      runnerSetNamespace: config.value.runnerSetNamespace,
      runnerSetName: config.value.runnerSetName,
      githubOwner: config.value.githubOwner,
      githubRepo: config.value.githubRepo,
      workflowFilter: config.value.workflowFilter,
      branchFilter: config.value.branchFilter,
      filePatterns: config.value.decodedFilePatterns || [],
      enabled: config.value.enabled,
      displaySettings: {
        defaultChartGroupMode: displaySettingsForm.defaultChartGroupMode,
        defaultChartGroupBy: displaySettingsForm.defaultChartGroupMode === 'dimension' ? displaySettingsForm.defaultChartGroupBy : undefined,
        defaultChartType: displaySettingsForm.defaultChartType,
        showRawDataByDefault: displaySettingsForm.showRawDataByDefault
      }
    })
    
    // Update local config with decoded object
    config.value.displaySettings = {
      defaultChartGroupMode: displaySettingsForm.defaultChartGroupMode,
      defaultChartGroupBy: displaySettingsForm.defaultChartGroupBy,
      defaultChartType: displaySettingsForm.defaultChartType,
      showRawDataByDefault: displaySettingsForm.showRawDataByDefault
    }
    
    ElMessage.success('Display settings saved')
  } catch (error) {
    console.error('Failed to save display settings:', error)
    ElMessage.error('Failed to save display settings')
  } finally {
    savingDisplaySettings.value = false
  }
}

// Basic Info Edit Functions
const startEditBasicInfo = () => {
  if (!config.value) return
  
  // Populate form with current config values
  basicInfoForm.name = config.value.name || ''
  basicInfoForm.description = config.value.description || ''
  basicInfoForm.githubOwner = config.value.githubOwner || ''
  basicInfoForm.githubRepo = config.value.githubRepo || ''
  basicInfoForm.runnerSetNamespace = config.value.runnerSetNamespace || ''
  basicInfoForm.runnerSetName = config.value.runnerSetName || ''
  basicInfoForm.workflowFilter = config.value.workflowFilter || ''
  basicInfoForm.branchFilter = config.value.branchFilter || ''
  basicInfoForm.enabled = config.value.enabled ?? true
  
  // Copy file patterns, ensure at least one empty pattern for editing
  const patterns = config.value.decodedFilePatterns || []
  basicInfoForm.filePatterns = patterns.length > 0 ? [...patterns] : ['']
  
  isEditingBasicInfo.value = true
}

const cancelEditBasicInfo = () => {
  isEditingBasicInfo.value = false
}

const addFilePattern = () => {
  basicInfoForm.filePatterns.push('')
}

const removeFilePattern = (index: number) => {
  if (basicInfoForm.filePatterns.length > 1) {
    basicInfoForm.filePatterns.splice(index, 1)
  }
}

const saveBasicInfo = async () => {
  if (!config.value) return
  
  // Validate required fields
  if (!basicInfoForm.name.trim()) {
    ElMessage.warning('Name is required')
    return
  }
  if (!basicInfoForm.githubOwner.trim()) {
    ElMessage.warning('GitHub Owner is required')
    return
  }
  if (!basicInfoForm.githubRepo.trim()) {
    ElMessage.warning('GitHub Repo is required')
    return
  }
  if (!basicInfoForm.runnerSetNamespace.trim()) {
    ElMessage.warning('Runner Set Namespace is required')
    return
  }
  if (!basicInfoForm.runnerSetName.trim()) {
    ElMessage.warning('Runner Set Name is required')
    return
  }
  
  // Filter out empty patterns
  const validPatterns = basicInfoForm.filePatterns.filter(p => p.trim())
  
  savingBasicInfo.value = true
  try {
    // Preserve existing displaySettings
    const displaySettings = typeof config.value.displaySettings === 'string' 
      ? decodeDisplaySettings(config.value.displaySettings)
      : config.value.displaySettings
    
    await updateConfigApi(config.value.id, {
      name: basicInfoForm.name.trim(),
      description: basicInfoForm.description.trim() || undefined,
      githubOwner: basicInfoForm.githubOwner.trim(),
      githubRepo: basicInfoForm.githubRepo.trim(),
      runnerSetNamespace: basicInfoForm.runnerSetNamespace.trim(),
      runnerSetName: basicInfoForm.runnerSetName.trim(),
      workflowFilter: basicInfoForm.workflowFilter.trim() || undefined,
      branchFilter: basicInfoForm.branchFilter.trim() || undefined,
      filePatterns: validPatterns,
      enabled: basicInfoForm.enabled,
      displaySettings
    })
    
    // Update local config
    config.value.name = basicInfoForm.name.trim()
    config.value.description = basicInfoForm.description.trim()
    config.value.githubOwner = basicInfoForm.githubOwner.trim()
    config.value.githubRepo = basicInfoForm.githubRepo.trim()
    config.value.runnerSetNamespace = basicInfoForm.runnerSetNamespace.trim()
    config.value.runnerSetName = basicInfoForm.runnerSetName.trim()
    config.value.workflowFilter = basicInfoForm.workflowFilter.trim()
    config.value.branchFilter = basicInfoForm.branchFilter.trim()
    config.value.decodedFilePatterns = validPatterns
    config.value.enabled = basicInfoForm.enabled
    
    ElMessage.success('Configuration saved')
    isEditingBasicInfo.value = false
  } catch (error) {
    console.error('Failed to save configuration:', error)
    ElMessage.error('Failed to save configuration')
  } finally {
    savingBasicInfo.value = false
  }
}

const fetchRuns = async () => {
  runsLoading.value = true
  try {
    let res: { runs: WorkflowRun[]; total: number }

    if (isRunnerSetCentric.value && runnerSetId.value) {
      // Runner-set-centric: fetch runs by runner set ID
      res = await getRunsByRunnerSetId(runnerSetId.value, {
        offset: (runsPagination.page - 1) * runsPagination.pageSize,
        limit: runsPagination.pageSize,
        status: runsFilter.status,
        trigger_source: runsFilter.triggerSource
      })
    } else if (isLegacyRunnerSetMode.value) {
      // Legacy mode: fetch runs by runner set name
      res = await getRunsByRunnerSet(runnerSetInfo.value.name, {
        offset: (runsPagination.page - 1) * runsPagination.pageSize,
        limit: runsPagination.pageSize,
        status: runsFilter.status
      })
    } else {
      // Config-centric mode
      res = await getRunsByConfig(configId.value, {
        offset: (runsPagination.page - 1) * runsPagination.pageSize,
        limit: runsPagination.pageSize,
        status: runsFilter.status,
        triggerSource: runsFilter.triggerSource
      })
    }

    runs.value = res.runs || []
    runsPagination.total = res.total || 0

    // Update stats from fetched runs or from runner set stats
    if (runnerSetStats.value) {
      runStats.pending = runnerSetStats.value.pending
      runStats.completed = runnerSetStats.value.completed
      runStats.failed = runnerSetStats.value.failed
    } else {
      runStats.pending = runs.value.filter(r => r.status === 'pending').length
      runStats.completed = runs.value.filter(r => r.status === 'completed').length
      runStats.failed = runs.value.filter(r => r.status === 'failed').length
    }
  } catch (error) {
    console.error('Failed to fetch runs:', error)
    ElMessage.error('Failed to load runs')
  } finally {
    runsLoading.value = false
  }
}

// Handle table filter changes
const handleFilterChange = (filters: Record<string, string[]>) => {
  // Update status filter
  if (filters.status && filters.status.length > 0) {
    runsFilter.status = filters.status[0]
  } else {
    runsFilter.status = undefined
  }
  
  // Update trigger filter
  if (filters.triggerSource && filters.triggerSource.length > 0) {
    runsFilter.triggerSource = filters.triggerSource[0]
  } else {
    runsFilter.triggerSource = undefined
  }
  
  // Reset to page 1 when filters change
  runsPagination.page = 1
  
  // Fetch data with new filters
  fetchRuns()
}

const loadSchemaVersions = async () => {
  schemasLoading.value = true
  try {
    const actualConfigId = effectiveConfigId.value
    if (!actualConfigId) {
      console.warn('[Schema] No config ID available, skipping schema load')
      return
    }
    const [schemasRes, changesRes] = await Promise.all([
      getSchemasByConfig(actualConfigId),
      getSchemaChanges(actualConfigId).catch(() => ({ changes: [] }))
    ])
    
    // Decode base64 encoded fields
    const decodedSchemas = (schemasRes.schemas || []).map(schema => ({
      ...schema,
      dimensionFields: decodeJsonArrayField(schema.dimensionFields as unknown as string),
      metricFields: decodeJsonArrayField(schema.metricFields as unknown as string),
      dateColumns: decodeJsonArrayField(schema.dateColumns as unknown as string)
    }))
    
    // Sort: active schema first, then by version descending
    schemaVersions.value = decodedSchemas.sort((a, b) => {
      // Active schema always comes first
      if (a.isActive && !b.isActive) return -1
      if (!a.isActive && b.isActive) return 1
      // Then sort by version descending
      return b.version - a.version
    })
    schemaChanges.value = changesRes.changes || []
    
    // Find active schema version, fallback to first in list
    const activeSchema = decodedSchemas.find(s => s.isActive)
    currentSchemaVersion.value = activeSchema?.version || schemasRes.currentVersion || (schemaVersions.value[0]?.version ?? null)
    
    // Set default schema version to active schema (or first if no active)
    if (!queryForm.schemaVersion && schemaVersions.value.length > 0) {
      queryForm.schemaVersion = activeSchema?.version || schemaVersions.value[0].version
    }
    
  } catch (error: any) {
    console.error('[Schema] Failed to load schema versions:', error)
    // Check if it's a 404 or API not implemented error
    if (error?.response?.status === 404) {
      console.warn('[Schema] Schema API not found - backend may not have implemented this endpoint yet')
    } else {
      ElMessage.warning('Failed to load schema versions - this feature may not be available yet')
    }
  } finally {
    schemasLoading.value = false
  }
}

const loadMetadata = async (schemaId?: number) => {
  try {
    const actualConfigId = effectiveConfigId.value
    if (!actualConfigId) {
      console.warn('[Metadata] No config ID available, skipping metadata load')
      return
    }
    
    // If we have a specific schema, use its fields directly
    const schema = schemaId 
      ? schemaVersions.value.find(s => s.id === schemaId)
      : schemaVersions.value[0]
    
    if (schema) {
      // Use schema's own fields - this is the source of truth
      availableDimensions.value = schema.dimensionFields || []
      availableMetrics.value = schema.metricFields || []
    } else {
      // Fallback to API if no schema found
      const fieldsRes = await getMetricFields(actualConfigId, schemaId ? { schemaId } : undefined)
      availableDimensions.value = fieldsRes.dimensionFields || []
      availableMetrics.value = fieldsRes.metricFields || []
    }
    
    // Load dimension values from API for filter dropdowns
    await loadDimensionValues(schemaId)
    
    // Set default metrics based on chart group mode
    if (availableMetrics.value.length > 0) {
      if (queryForm.chartGroupMode === 'metric') {
        // For metric group mode, select all metrics
        queryForm.selectedMetrics = [...availableMetrics.value]
      } else if (queryForm.selectedMetrics.length === 0) {
        // For other modes, only set default if none selected
        queryForm.selectedMetrics = [availableMetrics.value[0]]
      }
    }
  } catch (error) {
    console.error('Failed to load metadata:', error)
  }
}

// Load only dimension values (for filter dropdowns)
const loadDimensionValues = async (schemaId?: number) => {
  try {
    const actualConfigId = effectiveConfigId.value
    if (!actualConfigId) return
    
    const params = schemaId ? { schemaId } : undefined
    const dimsRes = await getDimensions(actualConfigId, params)
    // API returns { dimensions: { "Framework": [...], "GPU": [...] } }
    // Use dimensions field (the actual dimension values map)
    dimensionValues.value = dimsRes.dimensions || dimsRes.values || {}
  } catch (error) {
    console.error('Failed to load dimension values:', error)
  }
}

const updateConfig = async () => {
  if (!config.value) return
  try {
    // Backend requires all fields for update
    // Ensure displaySettings is an object (it might be base64 string from backend)
    const displaySettings = typeof config.value.displaySettings === 'string' 
      ? decodeDisplaySettings(config.value.displaySettings)
      : config.value.displaySettings
    
    await updateConfigApi(config.value.id, {
      name: config.value.name,
      description: config.value.description,
      runnerSetNamespace: config.value.runnerSetNamespace,
      runnerSetName: config.value.runnerSetName,
      githubOwner: config.value.githubOwner,
      githubRepo: config.value.githubRepo,
      workflowFilter: config.value.workflowFilter,
      branchFilter: config.value.branchFilter,
      filePatterns: config.value.decodedFilePatterns || [],
      enabled: config.value.enabled,
      displaySettings
    })
    ElMessage.success('Config updated')
  } catch (error) {
    console.error('Failed to update config:', error)
    ElMessage.error('Failed to update config')
  }
}

const retryRun = async (run: WorkflowRun) => {
  // TODO: Implement retry API
  ElMessage.info('Retry functionality coming soon')
}

const getStatusType = (status: string) => {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'pending': return 'warning'
    case 'collecting': return 'primary'
    default: return 'info'
  }
}

// Check if workflow is currently running
const isWorkflowRunning = (row: WorkflowRun) => {
  const status = row.workflowStatus || row.status
  return status === 'in_progress' || status === 'queued' || 
         status === 'workload_running' || status === 'workload_pending' ||
         status === 'pending' || status === 'collecting'
}

// Get workflow status display text
const getWorkflowStatusText = (row: WorkflowRun) => {
  // If we have the new workflow status, use it
  if (row.workflowStatus) {
    if (row.workflowStatus === 'completed' && row.workflowConclusion) {
      return row.workflowConclusion
    }
    return row.workflowStatus.replace(/_/g, ' ')
  }
  // Fallback to legacy status mapping
  const status = row.status
  if (status === 'workload_running' || status === 'collecting') return 'in progress'
  if (status === 'workload_pending' || status === 'pending') return 'queued'
  if (status === 'completed' || status === 'failed') return status
  return status?.replace(/_/g, ' ') || '-'
}

// Get workflow status tag type
const getWorkflowStatusType = (row: WorkflowRun) => {
  const conclusion = row.workflowConclusion
  const status = row.workflowStatus || row.status
  
  // If completed, use conclusion for color
  if (conclusion) {
    switch (conclusion) {
      case 'success': return 'success'
      case 'failure': return 'danger'
      case 'cancelled': return 'info'
      case 'skipped': return 'info'
      default: return 'info'
    }
  }
  
  // Use status for color
  switch (status) {
    case 'in_progress':
    case 'workload_running':
    case 'collecting':
      return 'warning'
    case 'queued':
    case 'pending':
    case 'workload_pending':
      return 'info'
    case 'completed':
      return 'success'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
}

// Get collection status display text
const getCollectionStatusText = (row: WorkflowRun) => {
  // If we have the new collection status, use it
  if (row.collectionStatus) {
    return row.collectionStatus
  }
  // Fallback to legacy status mapping
  const status = row.status
  if (status === 'workload_running' || status === 'workload_pending') return 'pending'
  if (status === 'collecting') return 'collecting'
  if (status === 'completed') return 'completed'
  if (status === 'failed') return 'failed'
  return status || '-'
}

// Get collection status tag type
const getCollectionStatusType = (row: WorkflowRun) => {
  const status = row.collectionStatus || row.status
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'collecting': return 'primary'
    case 'pending':
    case 'workload_running':
    case 'workload_pending':
      return 'info'
    case 'skipped': return 'info'
    default: return 'info'
  }
}

const getGithubRunUrl = (run: WorkflowRun) => {
  if (!run.githubRunId) return '#'
  
  // Use config if available, otherwise use runnerSetInfo
  const owner = config.value?.githubOwner || runnerSetInfo.value.githubOwner
  const repo = config.value?.githubRepo || runnerSetInfo.value.githubRepo
  
  if (!owner || !repo) return '#'
  return `https://github.com/${owner}/${repo}/actions/runs/${run.githubRunId}`
}

const getCommitUrl = (run: WorkflowRun) => {
  if (!run.headSha) return '#'
  
  // Use config if available, otherwise use runnerSetInfo
  const owner = config.value?.githubOwner || runnerSetInfo.value.githubOwner
  const repo = config.value?.githubRepo || runnerSetInfo.value.githubRepo
  
  if (!owner || !repo) return '#'
  return `https://github.com/${owner}/${repo}/commit/${run.headSha}`
}

const isValidDate = (dateStr: string) => {
  if (!dateStr) return false
  // Check for zero date (Go's time.Time zero value)
  if (dateStr.startsWith('0001-01-01') || dateStr === '0001-01-01T00:00:00Z') return false
  const date = dayjs(dateStr)
  return date.isValid() && date.year() > 1970
}

const executeQuery = async () => {
  if (!queryForm.selectedMetrics.length) {
    ElMessage.warning('Please select at least one metric')
    return
  }

  const actualConfigId = effectiveConfigId.value
  if (!actualConfigId) {
    ElMessage.warning('No config available for metrics query')
    return
  }

  querying.value = true
  rawResults.value = []
  dimensionGroups.value = []
  totalRawRecords.value = 0
  visibleSeries.value = new Set()
  seriesColors.value = {}

  try {
    const dimensions: Record<string, any> = {}
    for (const [key, values] of Object.entries(queryForm.dimensions)) {
      if (values && values.length > 0) {
        dimensions[key] = values
      }
    }

    const [rawRes, trendsRes] = await Promise.all([
      queryMetrics(actualConfigId, {
        start: queryForm.timeRange[0],
        end: queryForm.timeRange[1],
        schemaId: selectedSchemaId.value ?? undefined,
        dimensions,
        offset: 0,
        limit: queryForm.limit
      }),
      getMetricsTrends(actualConfigId, {
        start: queryForm.timeRange[0],
        end: queryForm.timeRange[1],
        schemaId: selectedSchemaId.value ?? undefined,
        dimensions,
        metricFields: queryForm.selectedMetrics,
        interval: queryForm.interval,
        groupBy: availableDimensions.value
      })
    ])

    rawResults.value = rawRes.metrics || []
    totalRawRecords.value = rawRes.total || 0

    // Process trends
    const groupsMap = new Map<string, DimensionGroup>()
    const timestamps = trendsRes.timestamps || []

    if (trendsRes.series && trendsRes.series.length > 0) {
      for (const series of trendsRes.series) {
        const allDims = (series.dimensions || {}) as Record<string, string>
        const dateValue = allDims.date
        const dims: Record<string, string> = {}
        for (const [k, v] of Object.entries(allDims)) {
          if (k !== 'date') {
            dims[k] = v
          }
        }
        
        const key = createDimensionKey(dims)
        
        if (!groupsMap.has(key)) {
          const group: DimensionGroup = {
            key,
            dimensions: dims,
            stats: {},
            count: 0,
            data: {},
            ...dims
          }
          groupsMap.set(key, group)
        }

        const group = groupsMap.get(key)!
        const metric = series.field || series.name
        
        if (!group.data[metric]) {
          group.data[metric] = []
        }
        
        const values = series.values || []
        
        if (dateValue) {
          const avgValue = values.length > 0 
            ? values.reduce((a: number, b: number) => a + b, 0) / values.length 
            : 0
          
          let matchedTimestamp = timestamps.find(t => t.includes(dateValue))
          if (!matchedTimestamp) {
            matchedTimestamp = `${dateValue}T00:00:00Z`
          }
          
          const existingPoint = group.data[metric].find(p => p.timestamp === matchedTimestamp)
          if (!existingPoint) {
            group.data[metric].push({
              timestamp: matchedTimestamp,
              value: avgValue
            })
          }
        } else {
          for (let i = 0; i < Math.min(timestamps.length, values.length); i++) {
            if (values[i] !== null && values[i] !== undefined) {
              group.data[metric].push({
                timestamp: timestamps[i],
                value: values[i]
              })
            }
          }
        }
        
        const allValues = group.data[metric].map(p => p.value).filter(v => v !== null && v !== undefined)
        if (allValues.length > 0) {
          group.stats[metric] = {
            avg: allValues.reduce((a, b) => a + b, 0) / allValues.length,
            sum: allValues.reduce((a, b) => a + b, 0),
            min: Math.min(...allValues),
            max: Math.max(...allValues)
          }
        }
        
        const seriesTotal = series.counts?.reduce((a, b) => a + b, 0) || allValues.length
        group.count = Math.max(group.count, seriesTotal)
      }
      
      for (const group of groupsMap.values()) {
        for (const metric of Object.keys(group.data)) {
          group.data[metric].sort((a, b) => 
            dayjs(a.timestamp).valueOf() - dayjs(b.timestamp).valueOf()
          )
        }
      }
    }

    dimensionGroups.value = Array.from(groupsMap.values())

    dimensionGroups.value.forEach((group, i) => {
      seriesColors.value[group.key] = COLORS[i % COLORS.length]
      visibleSeries.value.add(group.key)
    })

    await nextTick()
    renderAllCharts()

  } catch (error) {
    console.error('Failed to execute query:', error)
    ElMessage.error('Failed to execute query')
  } finally {
    querying.value = false
  }
}

const createDimensionKey = (dims: Record<string, string>): string => {
  const entries = Object.entries(dims).sort((a, b) => a[0].localeCompare(b[0]))
  return entries.map(([k, v]) => `${k}=${v}`).join('|')
}

const toggleSeries = (key: string) => {
  const newSet = new Set(visibleSeries.value)
  if (newSet.has(key)) {
    newSet.delete(key)
  } else {
    newSet.add(key)
  }
  visibleSeries.value = newSet
}

const toggleAllSeries = (visible: boolean) => {
  if (visible) {
    visibleSeries.value = new Set(dimensionGroups.value.map(g => g.key))
  } else {
    visibleSeries.value = new Set()
  }
}

const renderChart = () => {
  if (!chartRef.value) return
  
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }

  const visibleGroups = dimensionGroups.value.filter(g => visibleSeries.value.has(g.key))
  
  if (visibleGroups.length === 0) {
    chartInstance.clear()
    return
  }

  const timestampSet = new Set<string>()
  for (const group of visibleGroups) {
    for (const metric of queryForm.selectedMetrics) {
      const data = group.data[metric] || []
      for (const point of data) {
        timestampSet.add(point.timestamp)
      }
    }
  }
  
  // Sort timestamps using dayjs for reliable cross-browser date parsing
  const timestamps = Array.from(timestampSet).sort((a, b) => 
    dayjs(a).valueOf() - dayjs(b).valueOf()
  )
  
  const series: echarts.SeriesOption[] = []
  
  for (const group of visibleGroups) {
    for (const metric of queryForm.selectedMetrics) {
      const data = group.data[metric] || []
      const dataMap = new Map(data.map(p => [p.timestamp, p.value]))
      
      const dimLabel = Object.entries(group.dimensions)
        .map(([k, v]) => `${v}`)
        .join(', ')
      
      const seriesName = queryForm.selectedMetrics.length > 1 
        ? `${dimLabel} - ${metric}`
        : dimLabel

      series.push({
        name: seriesName,
        type: chartType.value,
        data: timestamps.map(t => dataMap.get(t) ?? null),
        smooth: chartType.value === 'line',
        itemStyle: {
          color: seriesColors.value[group.key]
        },
        lineStyle: chartType.value === 'line' ? {
          color: seriesColors.value[group.key]
        } : undefined
      })
    }
  }

  // Check if dark mode
  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const subtextColor = isDark ? '#A3A6AD' : '#606266'
  
  const option: echarts.EChartsOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: chartType.value === 'line' ? 'line' : 'shadow' },
      order: 'valueDesc',
      valueFormatter: (value) => {
        if (typeof value === 'number') {
          return value.toFixed(2)
        }
        return String(value)
      }
    },
    legend: {
      type: 'scroll',
      bottom: 0,
      data: series.map(s => s.name as string),
      textStyle: {
        color: textColor
      }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: timestamps.map(t => dayjs(t).format('YYYY-MM-DD')),
      axisLabel: {
        rotate: 30,
        interval: 'auto',
        color: subtextColor
      },
      axisLine: {
        lineStyle: {
          color: subtextColor
        }
      }
    },
    yAxis: {
      type: 'value',
      name: queryForm.selectedMetrics.join(', '),
      nameTextStyle: {
        color: subtextColor
      },
      axisLabel: {
        color: subtextColor,
        formatter: (value: number) => value.toFixed(2)
      },
      axisLine: {
        lineStyle: {
          color: subtextColor
        }
      },
      splitLine: {
        lineStyle: {
          color: isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.06)'
        }
      }
    },
    series
  }

  chartInstance.setOption(option, true)
}

const renderGroupedCharts = () => {
  // Dispose old instances
  groupedChartInstances.forEach(instance => instance.dispose())
  groupedChartInstances.length = 0

  if (!queryForm.chartGroupBy || groupedCharts.value.length === 0) return

  groupedCharts.value.forEach((chartGroup, idx) => {
    const el = groupedChartRefs.value[idx]
    if (!el) return

    const instance = echarts.init(el)
    groupedChartInstances.push(instance)

    const visibleGroups = chartGroup.groups.filter(g => visibleSeries.value.has(g.key))
    if (visibleGroups.length === 0) {
      instance.clear()
      return
    }

    const timestampSet = new Set<string>()
    for (const group of visibleGroups) {
      for (const metric of queryForm.selectedMetrics) {
        const data = group.data[metric] || []
        for (const point of data) {
          timestampSet.add(point.timestamp)
        }
      }
    }

    // Sort timestamps using dayjs for reliable cross-browser date parsing
    const timestamps = Array.from(timestampSet).sort((a, b) => 
      dayjs(a).valueOf() - dayjs(b).valueOf()
    )
    const series: echarts.SeriesOption[] = []

    for (const group of visibleGroups) {
      for (const metric of queryForm.selectedMetrics) {
        const data = group.data[metric] || []
        const dataMap = new Map(data.map(p => [p.timestamp, p.value]))

        // For grouped charts, exclude the groupBy dimension from label
        const dimLabel = Object.entries(group.dimensions)
          .filter(([k]) => k !== queryForm.chartGroupBy)
          .map(([, v]) => v)
          .join(', ') || metric

        const seriesName = queryForm.selectedMetrics.length > 1
          ? `${dimLabel} - ${metric}`
          : dimLabel

        series.push({
          name: seriesName,
          type: chartType.value,
          data: timestamps.map(t => dataMap.get(t) ?? null),
          smooth: chartType.value === 'line',
          itemStyle: {
            color: seriesColors.value[group.key]
          },
          lineStyle: chartType.value === 'line' ? {
            color: seriesColors.value[group.key]
          } : undefined
        })
      }
    }

    // Check if dark mode
    const isDark = document.documentElement.classList.contains('dark')
    const textColor = isDark ? '#E5EAF3' : '#303133'
    const subtextColor = isDark ? '#A3A6AD' : '#606266'
    
    const option: echarts.EChartsOption = {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: chartType.value === 'line' ? 'line' : 'shadow' },
        order: 'valueDesc',
        valueFormatter: (value) => {
          if (typeof value === 'number') {
            return value.toFixed(2)
          }
          return String(value)
        }
      },
      legend: {
        type: 'scroll',
        bottom: 0,
        data: series.map(s => s.name as string),
        textStyle: {
          color: textColor
        }
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '15%',
        top: '10%',
        containLabel: true
      },
      xAxis: {
        type: 'category',
        data: timestamps.map(t => dayjs(t).format('YYYY-MM-DD')),
        axisLabel: {
          rotate: 30,
          interval: 'auto',
          color: subtextColor
        },
        axisLine: {
          lineStyle: {
            color: subtextColor
          }
        }
      },
      yAxis: {
        type: 'value',
        name: queryForm.selectedMetrics.join(', '),
        nameTextStyle: {
          color: subtextColor
        },
        axisLabel: {
          color: subtextColor,
          formatter: (value: number) => value.toFixed(2)
        },
        axisLine: {
          lineStyle: {
            color: subtextColor
          }
        },
        splitLine: {
          lineStyle: {
            color: isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.06)'
          }
        }
      },
      series
    }

    instance.setOption(option, true)
  })
}

const renderAllCharts = () => {
  if (queryForm.chartGroupMode === 'dimension' && queryForm.chartGroupBy) {
    renderGroupedCharts()
  } else if (queryForm.chartGroupMode === 'metric') {
    renderMetricCharts()
  } else {
    renderChart()
  }
}

// Render charts grouped by metric
const renderMetricCharts = () => {
  // Dispose old instances
  metricChartInstances.forEach(instance => instance.dispose())
  metricChartInstances.length = 0

  if (queryForm.selectedMetrics.length === 0 || dimensionGroups.value.length === 0) return

  queryForm.selectedMetrics.forEach((metric, idx) => {
    const el = metricChartRefs.value[idx]
    if (!el) return

    const instance = echarts.init(el)
    metricChartInstances.push(instance)

    // Get all timestamps
    const allTimestamps = new Set<string>()
    dimensionGroups.value.forEach(group => {
      const data = group.data[metric] || []
      data.forEach(p => allTimestamps.add(p.timestamp))
    })
    // Sort timestamps using dayjs for reliable cross-browser date parsing
    const sortedTimestamps = Array.from(allTimestamps).sort((a, b) => 
      dayjs(a).valueOf() - dayjs(b).valueOf()
    )

    // Build series for each dimension group (only for this metric)
    const series: any[] = []
    const visibleGroups = dimensionGroups.value.filter(g => visibleSeries.value.has(g.key))

    for (const group of visibleGroups) {
      const data = group.data[metric] || []
      const dataMap = new Map(data.map(p => [p.timestamp, p.value]))

      // Use all dimensions for label (since we're grouping by metric)
      const dimLabel = Object.values(group.dimensions).join(', ')

      series.push({
        name: dimLabel,
        type: chartType.value,
        smooth: chartType.value === 'line',
        showSymbol: false,
        data: sortedTimestamps.map(ts => dataMap.get(ts) ?? null),
        itemStyle: {
          color: seriesColors.value[group.key]
        },
        lineStyle: chartType.value === 'line' ? {
          width: 2
        } : undefined
      })
    }

    const option = {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        formatter: (params: any) => {
          if (!params || params.length === 0) return ''
          const timestamp = sortedTimestamps[params[0].dataIndex]
          let result = `<div style="font-weight: bold; margin-bottom: 4px;">${dayjs(timestamp).format('YYYY-MM-DD HH:mm')}</div>`
          for (const p of params) {
            if (p.value !== null && p.value !== undefined) {
              result += `<div style="display: flex; justify-content: space-between; gap: 20px;">
                <span>${p.marker} ${p.seriesName}</span>
                <span style="font-weight: bold;">${typeof p.value === 'number' ? p.value.toFixed(2) : p.value}</span>
              </div>`
            }
          }
          return result
        }
      },
      legend: {
        type: 'scroll',
        bottom: 0,
        data: series.map(s => s.name)
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '15%',
        top: '10%',
        containLabel: true
      },
      xAxis: {
        type: 'category',
        boundaryGap: chartType.value === 'bar',
        data: sortedTimestamps.map(ts => dayjs(ts).format('YYYY-MM-DD')),
        axisLabel: {
          rotate: 45
        }
      },
      yAxis: {
        type: 'value',
        name: metric,
        nameLocation: 'middle',
        nameGap: 50,
        axisLabel: {
          formatter: (value: number) => {
            if (Math.abs(value) >= 1000000) {
              return (value / 1000000).toFixed(1) + 'M'
            } else if (Math.abs(value) >= 1000) {
              return (value / 1000).toFixed(1) + 'K'
            }
            return value.toFixed(1)
          }
        }
      },
      series
    }

    instance.setOption(option, true)
  })
}

const triggerBackfillAction = async () => {
  if (!backfillForm.timeRange.length) {
    ElMessage.warning('Please select a time range')
    return
  }

  backfillLoading.value = true
  try {
    if (isRunnerSetCentric.value && runnerSetId.value) {
      // Runner-set-centric backfill
      await triggerBackfillByRunnerSetId(runnerSetId.value, {
        startTime: backfillForm.timeRange[0],
        endTime: backfillForm.timeRange[1],
        dryRun: backfillForm.dryRun
      })
    } else if (configId.value) {
      // Config-based backfill (legacy)
      await triggerBackfill(configId.value, {
        startTime: backfillForm.timeRange[0],
        endTime: backfillForm.timeRange[1],
        dryRun: backfillForm.dryRun
      })
    }
    ElMessage.success(backfillForm.dryRun ? 'Dry run completed' : 'Backfill started')
    showBackfillDialog.value = false
    fetchRuns()
  } catch (error) {
    console.error('Failed to trigger backfill:', error)
    ElMessage.error('Failed to trigger backfill')
  } finally {
    backfillLoading.value = false
  }
}

const exportCSV = () => {
  const data = rawResults.value
  if (!data.length) return

  // Use schema-specific dimensions and metrics instead of raw data keys
  const dimKeys = availableDimensions.value
  const metricKeys = availableMetrics.value
  const headers = [...dimKeys, ...metricKeys, 'sourceFile', 'collectedAt']
  const rows = data.map(row => [
    ...dimKeys.map(k => row.dimensions?.[k] || ''),
    ...metricKeys.map(k => row.metrics?.[k] || ''),
    row.sourceFile || '',
    row.collectedAt || ''
  ].join(','))

  const csv = [headers.join(','), ...rows].join('\n')

  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `metrics-${dayjs().format('YYYYMMDD-HHmmss')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

const formatDate = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm')
}

const formatDuration = (start: string, end: string) => {
  const diff = dayjs(end).diff(dayjs(start), 'second')
  if (diff < 60) return `${diff}s`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`
  return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`
}

const formatNumber = (num: number) => {
  if (num === undefined || num === null) return '-'
  if (typeof num !== 'number') return String(num)
  return num.toLocaleString(undefined, { maximumFractionDigits: 4 })
}

// Create Config Form Helpers
const addConfigPattern = () => {
  createConfigForm.filePatterns.push('')
}

const removeConfigPattern = (index: number) => {
  if (createConfigForm.filePatterns.length > 1) {
    createConfigForm.filePatterns.splice(index, 1)
  }
}

const resetCreateConfigForm = () => {
  createConfigForm.name = ''
  createConfigForm.description = ''
  createConfigForm.filePatterns = ['']
  createConfigForm.workflowFilter = ''
  createConfigForm.branchFilter = ''
  createConfigForm.enabled = true
}

const submitCreateConfig = async () => {
  if (!createConfigForm.name.trim()) {
    ElMessage.warning('Config name is required')
    return
  }

  const validPatterns = createConfigForm.filePatterns.filter(p => p.trim())
  if (validPatterns.length === 0) {
    ElMessage.warning('At least one file pattern is required')
    return
  }

  if (!runnerSetId.value) {
    ElMessage.error('Runner set ID is missing')
    return
  }

  creatingConfig.value = true
  try {
    await createConfigForRunnerSet(runnerSetId.value, {
      name: createConfigForm.name.trim(),
      description: createConfigForm.description.trim() || undefined,
      filePatterns: validPatterns,
      workflowFilter: createConfigForm.workflowFilter.trim() || undefined,
      branchFilter: createConfigForm.branchFilter.trim() || undefined,
      enabled: createConfigForm.enabled
    })

    ElMessage.success('Config created successfully')
    showCreateConfigDialog.value = false

    // Reload config
    await fetchConfigByRunnerSet()
    if (config.value) {
      await loadSchemaVersions()
      await loadMetadata(selectedSchemaId.value ?? undefined)
    }
  } catch (error) {
    console.error('Failed to create config:', error)
    ElMessage.error('Failed to create config')
  } finally {
    creatingConfig.value = false
  }
}
</script>

<style scoped lang="scss">
// Import shared statistics page styles
@import '@/styles/stats-layout.scss';

.detail-page {
  padding: 20px;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  position: relative;
  
  // Decorative background glow effect
  &::before {
    content: '';
    position: absolute;
    top: -50px;
    right: 15%;
    width: 450px;
    height: 450px;
    background: radial-gradient(circle, rgba(64, 158, 255, 0.07) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }
  
  &::after {
    content: '';
    position: absolute;
    bottom: 50px;
    left: 10%;
    width: 400px;
    height: 400px;
    background: radial-gradient(circle, rgba(103, 194, 58, 0.06) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }

  .page-header {
    display: flex;
    align-items: center;
    gap: 20px;
    margin-bottom: 20px;
    position: relative;
    z-index: 1;
    
    .page-title {
      margin: 0;
      font-size: 20px;
      font-weight: 600;
      background: linear-gradient(135deg, var(--el-text-color-primary) 0%, var(--el-text-color-regular) 100%);
      -webkit-background-clip: text;
      background-clip: text;
      display: flex;
      align-items: center;
      
      @media (min-width: 1920px) {
        font-size: 22px;
      }
    }
    
    // Back button enhancement
    :deep(.el-button) {
      transition: all 0.3s ease;
      
      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(64, 158, 255, 0.2);
      }
    }
  }
  
  .page-subtitle {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    color: var(--el-text-color-secondary);
    margin: -12px 0 20px 0;
    position: relative;
    z-index: 1;
    
    .separator {
      color: var(--el-border-color);
    }
  }

  .detail-tabs {
    width: 100%;
    height: calc(100vh - 200px);
    display: flex;
    flex-direction: column;
    position: relative;
    z-index: 1;
    
    :deep(.el-tabs__header) {
      flex-shrink: 0;
      margin-bottom: 16px;
      
      .el-tabs__nav-wrap {
        &::after {
          background: linear-gradient(
            90deg,
            transparent 0%,
            rgba(64, 158, 255, 0.1) 50%,
            transparent 100%
          );
        }
      }
      
      .el-tabs__item {
        font-weight: 500;
        font-size: 14px;
        transition: all 0.3s ease;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
        
        &:hover {
          color: var(--el-color-primary);
          transform: translateY(-1px);
        }
        
        &.is-active {
          color: var(--el-color-primary);
          font-weight: 600;
        }
      }
      
      .el-tabs__active-bar {
        background: linear-gradient(90deg, var(--el-color-primary), var(--el-color-primary-light-3));
        height: 3px;
      }
    }
    
    :deep(.el-tabs__content) {
      flex: 1;
      overflow: hidden;
      
      .el-tab-pane {
        height: 100%;
        display: flex;
        flex-direction: column;
      }
    }
    
    .tab-label {
      display: flex;
      align-items: center;
      gap: 6px;
      
      .tab-badge {
        margin-left: 4px;
      }
    }
  }

  .tab-content {
    flex: 1;
    overflow: auto;
    width: 100%;
    
    // Running Workflow Banner
    .running-banner {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 14px 20px;
      margin-bottom: 20px;
      background: linear-gradient(135deg, rgba(230, 162, 60, 0.12) 0%, rgba(245, 108, 108, 0.08) 100%);
      border-radius: 12px;
      border: 1px solid rgba(230, 162, 60, 0.3);
      animation: banner-glow 2s ease-in-out infinite;
      
      @keyframes banner-glow {
        0%, 100% { box-shadow: 0 0 0 0 rgba(230, 162, 60, 0.2); }
        50% { box-shadow: 0 0 20px 0 rgba(230, 162, 60, 0.15); }
      }
      
      .banner-content {
        display: flex;
        align-items: center;
        gap: 12px;
        flex-wrap: wrap;
        
        .pulse-dot {
          width: 10px;
          height: 10px;
          background: var(--el-color-warning);
          border-radius: 50%;
          animation: pulse 1.5s ease-in-out infinite;
        }
        
        .banner-text {
          font-size: 14px;
          color: var(--el-text-color-primary);
          
          strong {
            color: var(--el-color-warning);
          }
        }
        
        .running-items {
          display: flex;
          gap: 8px;
          flex-wrap: wrap;
          
          .running-tag {
            cursor: pointer;
            transition: all 0.2s;
            
            &:hover {
              transform: translateY(-2px);
              box-shadow: 0 2px 8px rgba(230, 162, 60, 0.3);
            }
          }
          
          .more-tag {
            background: transparent;
          }
        }
      }
    }
    
    .banner-fade-enter-active,
    .banner-fade-leave-active {
      transition: all 0.3s ease;
    }
    
    .banner-fade-enter-from,
    .banner-fade-leave-to {
      opacity: 0;
      transform: translateY(-10px);
    }
    
    @keyframes pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50% { opacity: 0.6; transform: scale(1.3); }
    }
    
    // Clickable table styles
    .clickable-table {
      :deep(.clickable-row) {
        cursor: pointer;
        transition: background-color 0.2s;
        
        &:hover {
          background-color: var(--el-fill-color-light) !important;
        }
      }
      
      :deep(.running-row) {
        background-color: rgba(230, 162, 60, 0.04) !important;
        
        &:hover {
          background-color: rgba(230, 162, 60, 0.08) !important;
        }
      }
    }
    
    // Analytics sub-navigation
    .analytics-nav {
      margin-bottom: 20px;
      padding-bottom: 16px;
      border-bottom: 1px solid var(--el-border-color-lighter);
      
      :deep(.el-radio-group) {
        display: flex;
        gap: 0;
        
        .el-radio-button {
          .el-radio-button__inner {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 10px 20px;
            font-weight: 500;
            transition: all 0.3s ease;
            
            @media (min-width: 1920px) {
              padding: 12px 24px;
              font-size: 15px;
            }
            
            .el-icon {
              font-size: 16px;
            }
          }
          
          &.is-active .el-radio-button__inner {
            background: linear-gradient(135deg, var(--el-color-primary), var(--el-color-primary-light-3));
            border-color: var(--el-color-primary);
            box-shadow: 0 2px 8px rgba(64, 158, 255, 0.3);
          }
        }
      }
    }
    
    .analytics-view {
      width: 100%;
    }
    
    .filter-bar {
      display: flex;
      gap: 12px;
      margin-bottom: 16px;
      align-items: center;
      justify-content: flex-end;

      .flex-1 {
        flex: 1;
      }
      
      // Button size adaptation for large screens
      :deep(.el-button) {
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
          padding: 10px 20px;
        }
      }
    }

    .pagination-container {
      display: flex;
      justify-content: flex-end;
      margin-top: 16px;
    }

    .workload-cell {
      .workload-name {
        font-weight: 500;
        color: var(--el-text-color-primary);
        display: block;
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
      .workload-ns {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        font-family: monospace;
        
        @media (min-width: 1920px) {
          font-size: 13px;
        }
      }
    }

    .text-muted {
      color: var(--el-text-color-placeholder);
    }

    .commit-link {
      display: inline-flex;
      align-items: center;
      gap: 4px;

      .commit-sha {
        font-family: 'SF Mono', Monaco, 'Courier New', monospace;
        font-size: 12px;
      }
    }

    .commit-branch {
      display: block;
      font-size: 11px;
      color: var(--el-text-color-secondary);
      margin-top: 2px;
      max-width: 140px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .workflow-link {
      display: inline-flex;
      align-items: center;
      gap: 4px;

      .run-number {
        font-family: 'SF Mono', Monaco, 'Courier New', monospace;
        font-size: 12px;
      }
    }
    
    // Status cell with running indicator
    .status-cell {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 4px;
      
      .el-tag.is-running {
        animation: status-pulse 2s ease-in-out infinite;
        
        .running-dot {
          display: inline-block;
          width: 6px;
          height: 6px;
          background: currentColor;
          border-radius: 50%;
          margin-right: 5px;
          animation: dot-blink 1s ease-in-out infinite;
        }
      }
      
      .progress-text {
        font-size: 11px;
        color: var(--el-text-color-secondary);
        font-family: 'SF Mono', Monaco, monospace;
      }
    }
    
    @keyframes status-pulse {
      0%, 100% { box-shadow: 0 0 0 0 rgba(230, 162, 60, 0.4); }
      50% { box-shadow: 0 0 8px 2px rgba(230, 162, 60, 0.2); }
    }
    
    @keyframes dot-blink {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.3; }
    }
  }

  // Glassmorphism card effect
  .glass-card {
    background: linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%);
    backdrop-filter: blur(16px) saturate(180%);
    -webkit-backdrop-filter: blur(16px) saturate(180%);
    border-radius: 15px !important;
    border: 1px solid rgba(255, 255, 255, 0.18);
    box-shadow: 0 8px 32px rgba(17, 24, 39, 0.06);
    transition: all 0.3s ease;
    position: relative;
    overflow: hidden;
    
    // Colorful gradient top border
    &::before {
      content: '';
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      height: 2px;
      background: linear-gradient(
        90deg,
        transparent 0%,
        rgba(64, 158, 255, 0.5) 20%,
        rgba(103, 194, 58, 0.5) 50%,
        rgba(245, 108, 108, 0.5) 80%,
        transparent 100%
      );
      opacity: 0.6;
    }
    
    &:hover {
      transform: translateY(-2px);
      box-shadow: 0 12px 40px rgba(64, 158, 255, 0.18);
      border-color: rgba(64, 158, 255, 0.35);
      
      &::before {
        opacity: 1;
      }
    }
  }
  
  .query-card {
    margin-bottom: 20px;
    border-radius: 15px;
    
    // Simple large screen font adaptation
    @media (min-width: 1920px) {
      :deep(.el-form-item__label) {
        font-size: 15px;
      }
      
      :deep(.el-input, .el-select, .el-button) {
        font-size: 15px;
      }
    }

    .dimension-filters {
      display: flex;
      flex-wrap: wrap;
      gap: 16px;

      .dimension-row {
        display: flex;
        align-items: center;
        gap: 8px;

        .dim-label {
          font-size: 13px;
          color: var(--el-text-color-secondary);
          min-width: 80px;
          
          @media (min-width: 1920px) {
            font-size: 14px;
          }
        }

        .dim-select {
          width: 200px;
        }
      }
    }
    
    .form-hint {
      font-size: 12px;
      color: var(--el-text-color-secondary);
      margin-left: 12px;
      
      @media (min-width: 1920px) {
        font-size: 13px;
      }
    }
  }

  .chart-controls {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 16px;
  }

  .runs-card, .chart-card, .groups-card, .data-card {
    margin-bottom: 20px;
    border-radius: 15px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      &.clickable {
        cursor: pointer;
        user-select: none;

        &:hover {
          .card-title {
            color: var(--el-color-primary);
          }
        }
      }

      .card-title {
        font-size: 16px;
        font-weight: 600;
        display: flex;
        align-items: center;
        gap: 8px;
        transition: color 0.2s;

        .collapse-icon {
          transition: transform 0.3s;
          
          &.expanded {
            transform: rotate(90deg);
          }
        }
      }

      .chart-count {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }

      .card-actions {
        display: flex;
        align-items: center;
        gap: 8px;
      }
    }

    &.grouped-chart {
      .card-title {
        gap: 12px;
      }
    }
  }

  .chart-container {
    height: 400px;
    width: 100%;
  }

  .group-by-select {
    width: 240px;
  }
  
  .group-mode-radio {
    margin-right: 12px;
  }
  
  // Schema version selector styles
  .schema-version-select {
    width: 320px;
    
    @media (min-width: 1920px) {
      width: 380px;
    }
  }
  
  .schema-option {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    
    .version-label {
      font-weight: 600;
      min-width: 32px;
    }
    
    .record-count {
      color: var(--el-text-color-secondary);
      font-size: 12px;
    }
    
    .date-range {
      color: var(--el-text-color-placeholder);
      font-size: 12px;
      margin-left: auto;
    }
    
    .active-tag, .wide-tag {
      margin-left: 4px;
    }
  }
  
  .info-icon {
    color: var(--el-text-color-secondary);
    cursor: help;
    
    &:hover {
      color: var(--el-color-primary);
    }
  }
  
  .schema-alert, .schema-warning-alert {
    margin-bottom: 16px;
    
    .schema-changes {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 8px;
      
      > span {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 13px;
      }
    }
  }
  
  .single-schema-info {
    display: flex;
    align-items: center;
    gap: 12px;
    
    .schema-meta {
      color: var(--el-text-color-secondary);
      font-size: 13px;
    }
  }
  
  .field-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    
    .field-tag {
      margin: 0;
    }
  }

  .color-dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    display: inline-block;
  }

  .settings-content {
    flex: 1;
    overflow: auto;
    width: 100%;
    
    .settings-card {
      margin-bottom: 20px;
      border-radius: 15px;
      
      // Simple large screen font adaptation
      @media (min-width: 1920px) {
        :deep(.el-form-item__label) {
          font-size: 15px;
        }
        
        :deep(.el-descriptions__label) {
          font-size: 15px;
        }
        
        :deep(.el-descriptions__content) {
          font-size: 15px;
        }
        
        .card-title {
          font-size: 17px;
        }
        
        .card-hint {
          font-size: 13px;
        }
      }
      
      .card-header {
        display: flex;
        align-items: center;
        gap: 16px;
        
        .card-title {
          font-size: 16px;
          font-weight: 600;
        }
        
        .card-hint {
          font-size: 12px;
          color: var(--el-text-color-secondary);
        }
        
        .card-actions {
          margin-left: auto;
        }
      }
      
      .display-select {
        width: 280px;
      }
    }
    
    .pattern-tag {
      margin-right: 8px;
      margin-bottom: 4px;
    }
    
    // Basic Info Edit Form
    .basic-info-form {
      .form-hint {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        margin-top: 4px;
        display: block;
      }
    }
    
    .file-patterns-editor {
      width: 100%;
      
      .pattern-item {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 8px;
        
        .pattern-input {
          flex: 1;
          max-width: 500px;
        }
      }
      
      .add-pattern-btn {
        margin-bottom: 8px;
      }
      
      .form-hint {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }

  
  .w-full {
    width: 100%;
  }

  // Patterns editor for create config dialog
  .patterns-editor {
    width: 100%;

    .pattern-row {
      display: flex;
      gap: 8px;
      margin-bottom: 8px;

      .el-input {
        flex: 1;
      }
    }

    .add-pattern-btn {
      width: 100%;
      border-style: dashed;
    }
  }
  
  // Dialog large screen adaptation
  .backfill-dialog {
    @media (min-width: 1920px) {
      :deep(.el-dialog__title) {
        font-size: 18px;
      }
      
      :deep(.el-form-item__label) {
        font-size: 15px;
      }
      
      :deep(.el-dialog__body) {
        font-size: 15px;
      }
    }
  }
  
  
  // Table styles enhancement
  :deep(.el-table) {
    font-size: 14px;
    
    @media (min-width: 1920px) {
      font-size: 15px;
    }
    
    td {
      padding: 14px 0;
      
      @media (min-width: 1920px) {
        padding: 16px 0;
      }
    }
    
    th {
      font-size: 14px;
      font-weight: 600;
      padding: 14px 0;
      
      @media (min-width: 1920px) {
        font-size: 15px;
        padding: 16px 0;
      }
    }
    
    .cell {
      padding-left: 12px;
      padding-right: 12px;
    }
  }
  
  // Tag components adaptation
  :deep(.el-tag) {
    font-size: 12px;
    height: 24px;
    line-height: 22px;
    padding: 0 9px;
    
    @media (min-width: 1920px) {
      font-size: 13px;
      height: 26px;
      line-height: 24px;
      padding: 0 11px;
    }
    
    &.el-tag--small {
      font-size: 12px;
      height: 22px;
      line-height: 20px;
      padding: 0 7px;
      
      @media (min-width: 1920px) {
        font-size: 13px;
        height: 24px;
        line-height: 22px;
        padding: 0 9px;
      }
    }
  }
}
</style>
