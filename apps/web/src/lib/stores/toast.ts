import { writable } from 'svelte/store';

export interface Toast {
  id: string;
  message: string;
  type: 'error' | 'success' | 'info';
}

function createToasts() {
  const { subscribe, update } = writable<Toast[]>([]);

  function add(message: string, type: Toast['type'] = 'error') {
    const id = Math.random().toString(36).slice(2, 9);
    update(list => [...list, { id, message, type }]);
    setTimeout(() => remove(id), 4500);
  }

  function remove(id: string) {
    update(list => list.filter(t => t.id !== id));
  }

  return { subscribe, add, remove };
}

export const toasts = createToasts();
