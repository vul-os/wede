import { Trash2, CheckCircle2, Circle } from 'lucide-react';

const PRIORITY_COLORS = {
  high:   'text-red-400',
  medium: 'text-yellow-400',
  low:    'text-green-400',
};

export default function TaskList({ tasks, onToggle, onDelete }) {
  if (tasks.length === 0) {
    return <p className="text-gray-600 text-sm mt-8 text-center">No tasks yet.</p>;
  }

  return (
    <ul className="space-y-2">
      {tasks.map(task => (
        <li
          key={task.id}
          className="flex items-center gap-3 rounded-lg border border-gray-800 bg-gray-900 px-4 py-3"
        >
          <button
            onClick={() => onToggle(task.id)}
            className="shrink-0 text-gray-500 hover:text-indigo-400 transition-colors"
            aria-label={task.done ? 'Mark open' : 'Mark done'}
          >
            {task.done
              ? <CheckCircle2 size={18} className="text-indigo-500" />
              : <Circle size={18} />}
          </button>

          <span className={`flex-1 text-sm ${task.done ? 'line-through text-gray-600' : 'text-gray-200'}`}>
            {task.title}
          </span>

          <span className={`text-xs font-medium ${PRIORITY_COLORS[task.priority] ?? 'text-gray-500'}`}>
            {task.priority}
          </span>

          <button
            onClick={() => onDelete(task.id)}
            className="shrink-0 text-gray-700 hover:text-red-400 transition-colors"
            aria-label="Delete task"
          >
            <Trash2 size={15} />
          </button>
        </li>
      ))}
    </ul>
  );
}
