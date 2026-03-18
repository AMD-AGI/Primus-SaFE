# Answer Feedback API Documentation

## Overview

The Answer Feedback API provides endpoints for users to submit feedback (like/dislike) on individual AI responses. This is a per-message feedback system that allows users to rate the quality and helpfulness of each AI answer.

## Features

- ✅ **Like/Dislike per message**: Users can upvote or downvote individual AI responses
- ✅ **Duplicate prevention**: Each user can only vote once per message
- ✅ **Vote modification**: Users can cancel their vote and change it
- ✅ **Soft delete**: Cancelled votes are preserved for audit trail
- ✅ **Admin review**: Downvotes require manual review, upvotes auto-resolve
- ✅ **Statistics**: Track feedback metrics and trends

## API Endpoints

Base path: `/api/v1/answer-feedback`

### 1. Submit Feedback

Submit an upvote or downvote for an AI answer.

**Endpoint:** `POST /api/v1/answer-feedback`

**Headers:**
- `userId`: User identifier (required)
- `userName`: User display name (optional)

**Request Body:**
```json
{
  "vote_type": "up",        // "up" or "down"
  "message_id": 123,        // ID of the AI message
  "reason": "string"        // Optional reason (recommended for downvotes)
}
```

**Response:**
```json
{
  "success": true,
  "message": "Feedback submitted successfully",
  "data": {
    "id": 1,
    "user_id": "user123",
    "user_name": "John Doe",
    "vote_type": "up",
    "message_id": 123,
    "status": "resolved",
    "created_at": "2024-01-01T12:00:00"
  }
}
```

**Status Codes:**
- `200 OK`: Feedback submitted successfully
- `400 Bad Request`: Invalid input or duplicate vote
- `500 Internal Server Error`: Server error

### 2. Cancel Vote

Cancel a user's existing vote on a message.

**Endpoint:** `POST /api/v1/answer-feedback/cancel`

**Headers:**
- `userId`: User identifier (required)

**Request Body:**
```json
{
  "message_id": 123
}
```

**Response:**
```json
{
  "success": true,
  "message": "Vote cancelled successfully"
}
```

**Status Codes:**
- `200 OK`: Vote cancelled or no vote found
- `500 Internal Server Error`: Server error

### 3. Get Feedback List

Retrieve a paginated list of feedback with optional filters.

**Endpoint:** `GET /api/v1/answer-feedback`

**Query Parameters:**
- `status`: Filter by status (`pending`, `resolved`, `ignored`)
- `vote_type`: Filter by vote type (`up`, `down`)
- `page`: Page number (default: 1)
- `page_size`: Items per page (default: 20, max: 100)

**Response:**
```json
{
  "success": true,
  "message": "Found 42 feedback items",
  "data": {
    "items": [
      {
        "id": 1,
        "user_id": "user123",
        "user_name": "John Doe",
        "vote_type": "down",
        "reason": "Incorrect information",
        "message_id": 456,
        "status": "pending",
        "created_at": "2024-01-01T12:00:00"
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 42
    }
  }
}
```

### 4. Get Feedback Statistics

Get aggregate statistics about feedback.

**Endpoint:** `GET /api/v1/answer-feedback/stats`

**Response:**
```json
{
  "success": true,
  "data": {
    "total": 150,
    "pending": 12,
    "resolved": 130,
    "ignored": 8,
    "upvotes": 120,
    "downvotes": 30
  }
}
```

### 5. Get Single Feedback

Retrieve detailed information about a specific feedback item.

**Endpoint:** `GET /api/v1/answer-feedback/{feedback_id}`

**Response:**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "user_id": "user123",
    "user_name": "John Doe",
    "vote_type": "down",
    "reason": "Needs more detail",
    "message_id": 789,
    "status": "pending",
    "resolved_by": null,
    "resolved_by_name": null,
    "resolved_at": null,
    "resolution_note": null,
    "created_at": "2024-01-01T12:00:00"
  }
}
```

### 6. Resolve Feedback (Admin)

Mark a feedback item as resolved or ignored.

**Endpoint:** `POST /api/v1/answer-feedback/{feedback_id}/resolve`

**Headers:**
- `userId`: Admin user identifier (required)
- `userName`: Admin user display name (optional)

**Request Body:**
```json
{
  "status": "resolved",     // "resolved" or "ignored"
  "note": "Fixed in next update"  // Optional resolution note
}
```

**Response:**
```json
{
  "success": true,
  "message": "Feedback resolved successfully"
}
```

## Workflow

### Upvote Workflow

```
User clicks upvote button
         ↓
Frontend calls: POST /api/v1/answer-feedback
         ↓
Backend validates (no duplicate vote)
         ↓
Create feedback (status = 'resolved')
         ↓
Return success response
         ↓
UI updates button to active state
```

### Downvote Workflow

```
User clicks downvote button
         ↓
Frontend shows reason modal
         ↓
User selects/enters reason
         ↓
Frontend calls: POST /api/v1/answer-feedback with reason
         ↓
Backend validates (no duplicate vote)
         ↓
Create feedback (status = 'pending')
         ↓
Return success response
         ↓
UI updates button to active state
         ↓
Admin reviews via management page
         ↓
Admin calls: POST /api/v1/answer-feedback/{id}/resolve
```

### Cancel Vote Workflow

```
User clicks same button again
         ↓
Frontend calls: POST /api/v1/answer-feedback/cancel
         ↓
Backend soft deletes feedback record
         ↓
Return success response
         ↓
UI resets button to default state
```

## Database Schema

```sql
CREATE TABLE IF NOT EXISTS answer_feedback (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL,
    user_name VARCHAR(200),
    vote_type VARCHAR(10) NOT NULL CHECK (vote_type IN ('up', 'down')),
    reason TEXT,
    message_id BIGINT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'resolved', 'ignored')),
    resolved_by VARCHAR(128),
    resolved_by_name VARCHAR(200),
    resolved_at TIMESTAMP,
    resolution_note TEXT,
    deleted BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);
```

## Error Handling

### Common Errors

1. **Duplicate Vote**
   - Status: `400 Bad Request`
   - Message: "You have already voted on this message. Cancel your vote first to change it."

2. **Invalid Vote Type**
   - Status: `400 Bad Request`
   - Message: "vote_type must be 'up' or 'down'"

3. **Invalid Status**
   - Status: `400 Bad Request`
   - Message: "status must be 'resolved' or 'ignored'"

4. **Feedback Not Found**
   - Status: `404 Not Found`
   - Message: "Feedback not found"

## Integration Example

### JavaScript/TypeScript

```javascript
// Submit upvote
async function submitUpvote(messageId) {
  const response = await fetch('/api/v1/answer-feedback', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'userId': getCurrentUserId(),
      'userName': getCurrentUserName()
    },
    body: JSON.stringify({
      vote_type: 'up',
      message_id: messageId
    })
  });
  
  return await response.json();
}

// Submit downvote with reason
async function submitDownvote(messageId, reason) {
  const response = await fetch('/api/v1/answer-feedback', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'userId': getCurrentUserId(),
      'userName': getCurrentUserName()
    },
    body: JSON.stringify({
      vote_type: 'down',
      message_id: messageId,
      reason: reason
    })
  });
  
  return await response.json();
}

// Cancel vote
async function cancelVote(messageId) {
  const response = await fetch('/api/v1/answer-feedback/cancel', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'userId': getCurrentUserId()
    },
    body: JSON.stringify({
      message_id: messageId
    })
  });
  
  return await response.json();
}

// Get statistics
async function getFeedbackStats() {
  const response = await fetch('/api/v1/answer-feedback/stats', {
    headers: {
      'userId': getCurrentUserId()
    }
  });
  
  return await response.json();
}
```

### Python

```python
import requests

class AnswerFeedbackClient:
    def __init__(self, base_url, user_id, user_name=None):
        self.base_url = base_url
        self.user_id = user_id
        self.user_name = user_name or "Anonymous"
    
    def submit_feedback(self, message_id, vote_type, reason=None):
        """Submit upvote or downvote"""
        url = f"{self.base_url}/api/v1/answer-feedback"
        headers = {
            "userId": self.user_id,
            "userName": self.user_name
        }
        payload = {
            "vote_type": vote_type,
            "message_id": message_id,
            "reason": reason
        }
        response = requests.post(url, json=payload, headers=headers)
        return response.json()
    
    def cancel_vote(self, message_id):
        """Cancel vote"""
        url = f"{self.base_url}/api/v1/answer-feedback/cancel"
        headers = {"userId": self.user_id}
        payload = {"message_id": message_id}
        response = requests.post(url, json=payload, headers=headers)
        return response.json()
    
    def get_stats(self):
        """Get feedback statistics"""
        url = f"{self.base_url}/api/v1/answer-feedback/stats"
        headers = {"userId": self.user_id}
        response = requests.get(url, headers=headers)
        return response.json()
```

## Best Practices

1. **Always provide reason for downvotes**: Helps improve AI quality
2. **Implement rate limiting**: Prevent abuse of the feedback system
3. **Cache user votes**: Store vote status locally to show UI state
4. **Batch statistics queries**: Use pagination for large datasets
5. **Monitor downvote trends**: Set up alerts for unusual patterns
6. **Regular review**: Admins should review pending downvotes regularly

## Security Considerations

1. **User Authentication**: Always validate user_id from headers
2. **Rate Limiting**: Implement per-user rate limits
3. **Input Validation**: Validate all input parameters
4. **SQL Injection**: Use parameterized queries (already implemented)
5. **Authorization**: Ensure only admins can resolve feedback

