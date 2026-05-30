import { writable } from 'svelte/store';
import type { User } from '$lib/api';

export const currentUser = writable<User | null>(null);
