# Asynchronous Mode API Documentation

## Overview

OpenSPM Registry now supports asynchronous package publishing, allowing clients to upload packages without waiting for processing to complete. This is particularly useful for large packages or when additional processing (validation, scanning, etc.) is required.

## Configuration

Enable async mode in `config.yml`:

```yaml
server:
  async:
    enabled: true        # Enable/disable async mode
    workers: 4          # Number of background workers
    max_queue_size: 100 # Maximum jobs in queue
    operation_ttl: 24   # Operation TTL in hours
```

## API Endpoints

### 1. Asynchronous Package Publishing

To publish a package asynchronously, include the `Prefer: respond-async` header:

```bash
PUT /{scope}/{package}/{version}
Headers:
  Prefer: respond-async
  Content-Type: multipart/form-data
  Accept: application/json

Response: 202 Accepted
Headers:
  Location: /{scope}/{package}/{version}/status/{operation-id}
  Content-Version: 1
  Content-Type: application/json

Body:
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "created_at": "2024-01-20T10:00:00Z"
}
```

### 2. Check Operation Status

Poll the status endpoint to check if processing is complete:

```bash
GET /{scope}/{package}/{version}/status/{operation-id}

Response: 200 OK
Headers:
  Content-Type: application/json
  Content-Version: 1

Body (Processing):
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "created_at": "2024-01-20T10:00:00Z"
}

Body (Completed):
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "created_at": "2024-01-20T10:00:00Z",
  "completed_at": "2024-01-20T10:01:00Z",
  "result": {
    "location": "/{scope}/{package}/{version}",
    "message": "Package published successfully"
  }
}

Body (Failed):
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "failed",
  "created_at": "2024-01-20T10:00:00Z",
  "completed_at": "2024-01-20T10:00:30Z",
  "error": {
    "code": "validation_failed",
    "message": "Package manifest is invalid"
  }
}
```

## Status Values

- `processing`: Operation is still in progress
- `completed`: Operation completed successfully
- `failed`: Operation failed with error

## Error Codes

Common error codes in failed operations:
- `package_exists`: Package version already exists
- `validation_failed`: Package validation failed
- `internal_error`: Internal server error

## Example Usage

### 1. Upload Package Asynchronously

```bash
curl -X PUT \
  -H "Prefer: respond-async" \
  -H "Accept: application/json" \
  -F "source-archive=@MyPackage-1.0.0.zip" \
  https://registry.example.com/myscope/MyPackage/1.0.0
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "created_at": "2024-01-20T10:00:00Z"
}
```

### 2. Poll for Status

```bash
curl https://registry.example.com/myscope/MyPackage/1.0.0/status/550e8400-e29b-41d4-a716-446655440000
```

### 3. Handle in Swift

```swift
// Example Swift code to handle async publishing
func publishPackageAsync(package: Data) async throws {
    var request = URLRequest(url: publishURL)
    request.httpMethod = "PUT"
    request.setValue("respond-async", forHTTPHeaderField: "Prefer")
    
    let (data, response) = try await URLSession.shared.upload(for: request, from: package)
    
    if response.statusCode == 202 {
        let operation = try JSONDecoder().decode(AsyncOperation.self, from: data)
        let statusURL = response.value(forHTTPHeaderField: "Location")!
        
        // Poll for completion
        while true {
            let status = try await checkStatus(url: statusURL)
            switch status.status {
            case "completed":
                print("Package published at: \(status.result!.location)")
                return
            case "failed":
                throw PublishError(status.error!)
            case "processing":
                try await Task.sleep(nanoseconds: 2_000_000_000) // 2 seconds
            }
        }
    }
}
```

## Notes

1. Operations are stored in memory by default. Restarting the server will lose operation history.
2. Operations older than the configured TTL are automatically cleaned up.
3. If async mode is disabled, the `Prefer: respond-async` header is ignored.
4. The synchronous mode remains the default if no `Prefer` header is provided.