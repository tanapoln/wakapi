# BigQuery Integration

Wakapi can automatically write heartbeat events to Google BigQuery for advanced analytics and long-term data warehousing.

## Features

- **Automatic sync**: All heartbeat events are automatically written to BigQuery
- **Schema matching**: BigQuery table schema matches 100% with PostgreSQL Heartbeat model
- **Graceful error handling**: BigQuery errors don't affect heartbeat ingestion
- **Asynchronous writes**: BigQuery writes happen in the background without blocking
- **Auto table creation**: The BigQuery table is created automatically if it doesn't exist

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

## BigQuery Schema

The BigQuery table has the following schema (matching the Heartbeat model):

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

## Example Queries

### Total coding time by user
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

### Top languages by user
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

### Daily activity heatmap
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

## Monitoring

Check the Wakapi logs for BigQuery integration status:

```bash
# On startup
[INFO] bigquery integration enabled project=your-project dataset=wakapi table=heartbeats

# On successful table creation
[INFO] bigquery table created successfully

# On errors
[ERROR] failed to insert heartbeats to bigquery error=<error message>
```

BigQuery errors are logged but don't affect normal Wakapi operation. Heartbeats will still be saved to the primary database even if BigQuery writes fail.

## Cost Considerations

- **Storage**: ~$0.02 per GB per month
- **Streaming inserts**: Free (using the BigQuery Storage Write API)
- **Query costs**: $5 per TB scanned

For a typical user generating 1000 heartbeats/day:
- ~1 MB/day
- ~365 MB/year
- Storage cost: ~$0.01/year
- Queries cost depends on usage

BigQuery offers a [free tier](https://cloud.google.com/bigquery/pricing#free-tier) with:
- 10 GB storage per month
- 1 TB queries per month

## Troubleshooting

### Table not found
The table is created automatically on first startup. Check logs for creation errors.

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
