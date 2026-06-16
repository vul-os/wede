import { useState, useEffect } from 'react';
import TaskList from './components/TaskList';
import TaskForm from './components/TaskForm';
import { fetchTasks, createTask, updateTask, deleteTask } from './utils/api';

export default function App() {
  const [tasks, setTasks] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filter, setFilter] = useState('all'); // 'all' | 'open' | 'done'

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      setLoading(true);
      const data = await fetchTasks();
      setTasks(data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate(title, priority) {
    const { id } = await createTask(title, priority);
    setTasks(prev => [{ id, title, done: false, priority, created_at: new Date().toISOString() }, ...prev]);
  }

  async function handleToggle(id) {
    const task = tasks.find(t => t.id === id);
    await updateTask(id, { done: !task.done });
    setTasks(prev => prev.map(t => t.id === id ? { ...t, done: !t.done } : t));
  }

  async function handleDelete(id) {
    await deleteTask(id);
    setTasks(prev => prev.filter(t => t.id !== id));
  }

  const visible = tasks.filter(t => {
    if (filter === 'open') return !t.done;
    if (filter === 'done') return t.done;
    return true;
  });

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100 px-4 py-8">
      <div className="max-w-xl mx-auto">
        <h1 className="text-2xl font-semibold mb-6">taskboard</h1>

        <TaskForm onSubmit={handleCreate} />

        <div className="flex gap-2 my-4 text-sm">
          {['all', 'open', 'done'].map(f => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-3 py-1 rounded-full border transition-colors ${
                filter === f
                  ? 'bg-indigo-600 border-indigo-600 text-white'
                  : 'border-gray-700 text-gray-400 hover:border-gray-500'
              }`}
            >
              {f}
            </button>
          ))}
        </div>

        {loading && <p className="text-gray-500 text-sm">Loading…</p>}
        {error && <p className="text-red-400 text-sm">Error: {error}</p>}
        {!loading && (
          <TaskList tasks={visible} onToggle={handleToggle} onDelete={handleDelete} />
        )}
      </div>
    </div>
  );
}
