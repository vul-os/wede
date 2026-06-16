# taskboard

A lightweight task management API with a React frontend.

## Stack

- **Backend**: Go (net/http), SQLite
- **Frontend**: React, Tailwind CSS
- **Auth**: JWT (HS256)

## Quick start

```bash
go run ./api/...
npm install && npm run dev
```

API runs on `http://localhost:8080`, frontend on `http://localhost:5173`.

## Project layout

```
taskboard/
├── api/
│   ├── main.go          # HTTP server entry point
│   ├── handlers.go      # Route handlers
│   └── middleware.go    # Auth middleware
├── src/
│   ├── App.jsx          # Root component
│   ├── components/
│   │   ├── TaskList.jsx
│   │   └── TaskForm.jsx
│   └── utils/
│       └── api.js       # Fetch helpers
├── tests/
│   └── handlers_test.go
└── package.json
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/login` | Get a JWT |
| `GET`  | `/tasks` | List tasks (auth required) |
| `POST` | `/tasks` | Create task |
| `PATCH`| `/tasks/:id` | Update task |
| `DELETE`| `/tasks/:id` | Delete task |

## License

MIT
