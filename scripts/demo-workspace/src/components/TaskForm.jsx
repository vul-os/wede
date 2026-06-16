import { useState } from 'react';
import { PlusCircle } from 'lucide-react';

export default function TaskForm({ onSubmit }) {
  const [title, setTitle] = useState('');
  const [priority, setPriority] = useState('medium');
  const [busy, setBusy] = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    const trimmed = title.trim();
    if (!trimmed) return;
    setBusy(true);
    try {
      await onSubmit(trimmed, priority);
      setTitle('');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex gap-2">
      <input
        className="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-3 py-2 text-sm text-gray-100
                   placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
        placeholder="New task…"
        value={title}
        onChange={e => setTitle(e.target.value)}
        disabled={busy}
      />
      <select
        value={priority}
        onChange={e => setPriority(e.target.value)}
        className="rounded-lg border border-gray-700 bg-gray-900 px-2 py-2 text-sm text-gray-400
                   focus:border-indigo-500 focus:outline-none"
        disabled={busy}
      >
        <option value="high">high</option>
        <option value="medium">medium</option>
        <option value="low">low</option>
      </select>
      <button
        type="submit"
        disabled={busy || !title.trim()}
        className="rounded-lg bg-indigo-600 px-3 py-2 text-sm font-medium text-white
                   hover:bg-indigo-500 disabled:opacity-40 transition-colors"
      >
        <PlusCircle size={16} />
      </button>
    </form>
  );
}
