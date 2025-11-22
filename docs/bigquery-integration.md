# BigQuery Integration

Wakapi can automatically write heartbeat events and computed durations to Google BigQuery for advanced analytics and long-term data warehousing.

## Features

- **Automatic sync**: All heartbeat events and computed durations are automatically written to BigQuery
- **Schema matching**: BigQuery table schemas match with PostgreSQL models (Heartbeat and Duration)
- **Graceful error handling**: BigQuery errors don't affect heartbeat ingestion or duration computation
- **Asynchronous writes**: BigQuery writes happen in the background without blocking
- **Auto table creation**: BigQuery tables are created automatically if they don't exist
- **Two tables**: 
  - `heartbeats` (or your configured table_id): Raw heartbeat data
  - `heartbeats_durations` (or your configured table_id + `_durations`): Computed duration data

## Configuration

Add the following configuration to your `config.yml`:

```yaml
bigquery:
  enabled: true                                          # Enable BigQuery integration
  service_account_json_path: /path/to/service-account.json  # Path to GCP service account JSON
  project_id: your-gcp-project-id                       # GCP project ID
  dataset_id: wakapi                                    # BigQuery dataset ID
  table_id: heartbeats                                  # BigQuery table ID
```

Or use environment variables:

```bash
export WAKAPI_BIGQUERY_ENABLED=true
export WAKAPI_BIGQUERY_SERVICE_ACCOUNT_JSON_PATH=/path/to/service-account.json
export WAKAPI_BIGQUERY_PROJECT_ID=your-gcp-project-id
export WAKAPI_BIGQUERY_DATASET_ID=wakapi
export WAKAPI_BIGQUERY_TABLE_ID=heartbeats
```

## GCP Setup

### 1. Create a BigQuery Dataset

```bash
# Using gcloud CLI
gcloud bigquery datasets create wakapi --location=US
```

Or via the [BigQuery Console](https://console.cloud.google.com/bigquery).

### 2. Create a Service Account

1. Go to [GCP Console - Service Accounts](https://console.cloud.google.com/iam-admin/serviceaccounts)
2. Click "Create Service Account"
3. Name: `wakapi-bigquery`
4. Grant roles:
   - `BigQuery Data Editor` - to write data
   - `BigQuery Job User` - to run queries
5. Create and download a JSON key file

### 3. Grant Permissions

```bash
# Grant BigQuery permissions to the service account
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:wakapi-bigquery@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/bigquery.dataEditor"

gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:wakapi-bigquery@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/bigquery.jobUser"
```

## BigQuery Schemas

### Heartbeats Table

The BigQuery heartbeats table has the following schema (matching the Heartbeat model):

| Field              | Type      | Description                    |
|--------------------|-----------|--------------------------------|
| id                 | INTEGER   | Unique heartbeat ID            |
| user_id            | STRING    | User identifier                |
| entity             | STRING    | File path or URL               |
| type               | STRING    | Type (file, domain, etc.)      |
| category           | STRING    | Category (coding, browsing)    |
| project            | STRING    | Project name                   |
| branch             | STRING    | Git branch                     |
| language           | STRING    | Programming language           |
| is_write           | BOOLEAN   | Whether it's a write operation |
| editor             | STRING    | Editor/IDE name                |
| operating_system   | STRING    | Operating system               |
| machine            | STRING    | Machine name                   |
| user_agent         | STRING    | User agent string              |
| time               | TIMESTAMP | Event timestamp                |
| hash               | STRING    | Heartbeat hash (unique)        |
| origin             | STRING    | Origin source                  |
| origin_id          | STRING    | Origin identifier              |
| created_at         | TIMESTAMP | Record creation time           |
| lines              | INTEGER   | Number of lines                |
| lineno             | INTEGER   | Line number                    |
| cursorpos          | INTEGER   | Cursor position                |
| line_deletions     | INTEGER   | Lines deleted                  |
| line_additions     | INTEGER   | Lines added                    |
| project_root_count | INTEGER   | Project root count             |

### Durations Table

The BigQuery durations table (`{table_id}_durations`) has the following schema (matching the Duration model):

| Field              | Type      | Description                          |
|--------------------|-----------|--------------------------------------|
| id                 | INTEGER   | Unique duration ID                   |
| user_id            | STRING    | User identifier                      |
| time               | TIMESTAMP | Start time of duration               |
| duration           | INTEGER   | Duration length in nanoseconds       |
| project            | STRING    | Project name                         |
| language           | STRING    | Programming language                 |
| editor             | STRING    | Editor/IDE name                      |
| operating_system   | STRING    | Operating system                     |
| machine            | STRING    | Machine name                         |
| category           | STRING    | Category (coding, browsing)          |
| branch             | STRING    | Git branch                           |
| entity             | STRING    | File path or URL (most prominent)    |
| num_heartbeats     | INTEGER   | Number of heartbeats in this duration|
| group_hash         | STRING    | Hash for grouping durations          |
| timeout            | INTEGER   | Heartbeat timeout used (nanoseconds) |

## Example Queries

### Heartbeat Queries

#### Total coding time by user
```sql
SELECT 
  user_id,
  COUNT(*) as heartbeats,
  DATE(time) as date
FROM `your-project.wakapi.heartbeats`
WHERE category = 'coding'
GROUP BY user_id, date
ORDER BY date DESC
```

#### Top languages by user
```sql
SELECT 
  user_id,
  language,
  COUNT(*) as heartbeats
FROM `your-project.wakapi.heartbeats`
WHERE language IS NOT NULL AND language != ''
GROUP BY user_id, language
ORDER BY heartbeats DESC
LIMIT 10
```

#### Daily activity heatmap
```sql
SELECT 
  DATE(time) as date,
  EXTRACT(HOUR FROM time) as hour,
  COUNT(*) as activity_count
FROM `your-project.wakapi.heartbeats`
WHERE user_id = 'your-user-id'
  AND time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY date, hour
ORDER BY date, hour
```

### Duration Queries

#### Total coding time by project (from durations)
```sql
SELECT 
  user_id,
  project,
  SUM(duration) / 1000000000 / 3600 as hours_coded,
  SUM(num_heartbeats) as total_heartbeats
FROM `your-project.wakapi.heartbeats_durations`
WHERE time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY user_id, project
ORDER BY hours_coded DESC
```

#### Daily coding time per language
```sql
SELECT 
  DATE(time) as date,
  language,
  SUM(duration) / 1000000000 / 3600 as hours_coded
FROM `your-project.wakapi.heartbeats_durations`
WHERE user_id = 'your-user-id'
  AND time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY)
GROUP BY date, language
ORDER BY date DESC, hours_coded DESC
```

#### Average session duration by editor
```sql
SELECT 
  editor,
  AVG(duration) / 1000000000 / 60 as avg_session_minutes,
  COUNT(*) as session_count
FROM `your-project.wakapi.heartbeats_durations`
WHERE user_id = 'your-user-id'
  AND time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY editor
ORDER BY avg_session_minutes DESC
```

#### Most active coding days
```sql
SELECT 
  DATE(time) as date,
  COUNT(DISTINCT project) as projects_worked_on,
  SUM(duration) / 1000000000 / 3600 as hours_coded,
  SUM(num_heartbeats) as total_heartbeats
FROM `your-project.wakapi.heartbeats_durations`
WHERE user_id = 'your-user-id'
  AND time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 180 DAY)
GROUP BY date
ORDER BY hours_coded DESC
LIMIT 10
```

## Monitoring

Check the Wakapi logs for BigQuery integration status:

```bash
# On startup
[INFO] bigquery service initialized successfully

# On successful table creation
[INFO] bigquery heartbeat table created successfully
[INFO] bigquery duration table created successfully

# On errors
[ERROR] failed to insert heartbeats to bigquery error=<error message>
[ERROR] failed to insert durations to bigquery error=<error message>
```

BigQuery errors are logged but don't affect normal Wakapi operation. Heartbeats will still be saved to the primary database even if BigQuery writes fail. Similarly, durations will be computed and cached even if BigQuery writes fail.

## Cost Considerations

- **Storage**: ~$0.02 per GB per month
- **Streaming inserts**: Free (using the BigQuery Storage Write API)
- **Query costs**: $5 per TB scanned

For a typical user generating 1000 heartbeats/day:
- Heartbeats: ~1 MB/day (~365 MB/year)
- Durations: ~0.1 MB/day (~36 MB/year) - much more compact than heartbeats
- Combined storage cost: ~$0.01/year
- Queries cost depends on usage

BigQuery offers a [free tier](https://cloud.google.com/bigquery/pricing#free-tier) with:
- 10 GB storage per month
- 1 TB queries per month

## Troubleshooting

### Tables not found
The tables are created automatically on first startup. Check logs for creation errors. You should see both:
- `{table_id}` table for heartbeats
- `{table_id}_durations` table for durations

### Permission denied
Ensure the service account has the correct IAM roles (`bigquery.dataEditor` and `bigquery.jobUser`).

### Invalid service account file
Verify the JSON file path is correct and the file is valid JSON.

### Failed to insert heartbeats
Check BigQuery quotas and ensure the dataset exists. View detailed errors in the Wakapi logs.

## Advanced Configuration

### Using Workload Identity (GKE)

If running on Google Kubernetes Engine, you can use Workload Identity instead of a service account JSON file:

```yaml
# Don't set service_account_json_path
# Instead, configure workload identity for your pod
```

### Custom Table Partitioning

For better query performance and cost optimization, consider partitioning the table by date:

```sql
CREATE TABLE `your-project.wakapi.heartbeats`
PARTITION BY DATE(time)
CLUSTER BY user_id, project
AS SELECT * FROM `your-project.wakapi.heartbeats`
```

## Security

- Store the service account JSON file securely
- Use least privilege principle (only grant necessary permissions)
- Rotate service account keys regularly
- Consider using Workload Identity on GKE
- Enable audit logging for BigQuery access
