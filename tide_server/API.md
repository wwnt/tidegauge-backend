# Guide to Accessing Sensor Data via API

This document provides a step-by-step guide on how to retrieve historical sensor data using the API. All APIs referenced are documented in the [Postman API Documentation](https://documenter.getpostman.com/view/1237417/2sAY547z2M).

---

## Prerequisites

Before calling any API to fetch sensor data, you need to authenticate and obtain an `access_token`. This is done by calling the `login` API.

### 1. Authentication: `login` API
- **Endpoint**: `/login`
- **Method**: `POST`
- **Parameters**:
    - `username`: Your username.
    - `password`: Your password.
- **Response**:
  ```json
  {
    "access_token": "access_token",
    "expires_in": 43200,
    "token_type": "bearer"
  }
  ```

Once you have the `access_token`, include it in the `Authorization` header for all subsequent API calls:
```
Authorization: Bearer <access_token>
```
If the token is invalid or expired, the API will return a 401 Unauthorized error.

---

## Fetching Historical Data

### 2. Retrieve Station IDs: `listStation` API
- **Endpoint**: `/listStation`
- **Method**: `GET`
- **Parameters**: None
- **Response**:
  ```json
  [
    {
      "id": "1048a910-2a2b-11eb-9abd-d89ef3266df6",
      "identifier": "station1",
      "name": "station1",
      "ip_addr": "192.168.1.38:57234",
      "location": {
        "name": "Ltest11",
        "position": "51.51557344578067,-0.10129627561850096"
      },
      "status": "Normal",
      "status_changed_at": 1731394041996,
      "upstream": false
    }
  ]
  ```

### 3. Retrieve Item Names: `listItem` API
- **Endpoint**: `/listItem`
- **Method**: `GET`
- **Parameters**:
    - `station_id` (optional): If provided, returns items for the specified station.
- **Response**:
  ```json
  [
    {
      "station_id": "1048a910-2a2b-11eb-9abd-d89ef3266df6",
      "name": "location1_rain_gauge",
      "type": "rain_gauge",
      "device_name": "地点1雨量",
      "status": "NoStatus",
      "status_changed_at": 1729589919395,
      "available": true
    }
  ]
  ```

### 4. Fetch Historical Data: `dataHistory` API
- **Endpoint**: `/dataHistory`
- **Method**: `POST`
- **Parameters**:
    - `station_id`: ID from `listStation`.
    - `item_name`: Item name from `listItem`.
    - `start`: Start timestamp (milliseconds).
    - `end`: End timestamp (milliseconds).
- **Response**:
  ```json
  [
    {
      "val": 0,
      "msec": 1724411007076
    },
    {
      "val": 0,
      "msec": 1724411097075
    }
  ]
  ```

---

## Example Workflow

1. **Login**:
   Request an `access_token` via the `login` API.
2. **Retrieve Station and Item Info**:
   Use `listStation` to get station IDs and `listItem` to get the item names.
3. **Query Historical Data**:
   Pass the station ID, item name, and time range to the `dataHistory` API.