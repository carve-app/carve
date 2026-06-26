<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { get } from 'svelte/store';
  import { fetchUser, fetchDueCount, logout as revokeSession } from '$lib/api';
  import { lang, LANG_LABELS, type LangCode } from '$lib/stores/lang';
  import { currentUser } from '$lib/stores/user';

  let dueCount = 0;
  let userMenuOpen = false;
  let sidebarOpen = false;
  let ready = false;
  let unsubLang: (() => void) | undefined;

  const NAV = [
    { href: '/review',   label: 'Review'   },
    { href: '/cards',    label: 'Cards'    },
    { href: '/grammar',  label: 'Grammar'  },
    { href: '/library',  label: 'Library'  },
    { href: '/discover', label: 'Discover' },
    { href: '/import',   label: 'Import'   },
    { href: '/decks',    label: 'Decks'    },
    { href: '/stats',    label: 'Stats'    },
    { href: '/output',   label: 'Output'   },
    { href: '/settings', label: 'Settings' },
    { href: '/export',   label: 'Export'   },
  ];

  function isActive(href: string): boolean {
    return $page.url.pathname === href || $page.url.pathname.startsWith(href + '/');
  }

  async function refreshDueCount(language: string) {
    try {
      const res = await fetchDueCount(language);
      dueCount = res.due_count;
    } catch {
      // non-fatal
    }
  }

  onMount(async () => {
    if (!localStorage.getItem('carve_access_token')) {
      goto('/login');
      return;
    }
    try {
      const user = await fetchUser();
      currentUser.set(user);
    } catch {
      localStorage.removeItem('carve_access_token');
      goto('/login');
      return;
    }
    ready = true;
    await refreshDueCount(get(lang));

    let init = false;
    unsubLang = lang.subscribe(l => { if (init) refreshDueCount(l); });
    init = true;
  });

  onDestroy(() => { unsubLang?.(); });

  function setLang(e: Event) {
    lang.set((e.target as HTMLSelectElement).value as LangCode);
  }

  async function logout() {
    try {
      await revokeSession();
    } finally {
      localStorage.removeItem('carve_access_token');
      currentUser.set(null);
      goto('/login');
    }
  }

  function closeSidebar() { sidebarOpen = false; }
</script>

{#if ready}
  <div class="shell" class:sidebar-open={sidebarOpen}>
    <aside class="sidebar">
      <div class="sidebar-brand">
        <a href="/" class="brand-link">Carve</a>
      </div>
      <nav class="sidebar-nav">
        {#each NAV as item}
          <a
            href={item.href}
            class="nav-item"
            class:active={isActive(item.href)}
            on:click={closeSidebar}
          >
            {item.label}
            {#if item.href === '/review' && dueCount > 0}
              <span class="due-badge">{dueCount > 999 ? '999+' : dueCount}</span>
            {/if}
          </a>
        {/each}
      </nav>
    </aside>

    {#if sidebarOpen}
      <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
      <div class="sidebar-overlay" on:click={closeSidebar}></div>
    {/if}

    <div class="shell-main">
      <header class="topbar">
        <button class="menu-btn" on:click={() => sidebarOpen = !sidebarOpen} aria-label="Toggle menu">
          ☰
        </button>
        <div class="topbar-right">
          <select class="lang-select" value={$lang} on:change={setLang} aria-label="Language">
            {#each Object.entries(LANG_LABELS) as [code, label]}
              <option value={code}>{label}</option>
            {/each}
          </select>

          <div class="user-menu-wrap">
            <button
              class="user-btn"
              aria-label="User menu"
              aria-haspopup="menu"
              aria-expanded={userMenuOpen}
              on:click|stopPropagation={() => userMenuOpen = !userMenuOpen}
            >
              {$currentUser?.display_name?.slice(0, 1).toUpperCase() ?? '?'}
            </button>
            {#if userMenuOpen}
              <div class="user-dropdown" role="menu">
                <div class="dropdown-info">
                  <div class="dropdown-name">{$currentUser?.display_name}</div>
                  <div class="dropdown-email">{$currentUser?.email}</div>
                </div>
                <hr class="dropdown-hr" />
                <a href="/settings" class="dropdown-item" role="menuitem" on:click={() => userMenuOpen = false}>Settings</a>
                <button class="dropdown-item logout-item" role="menuitem" on:click={logout}>Sign out</button>
              </div>
            {/if}
          </div>
        </div>
      </header>

      <div class="shell-content">
        <slot />
      </div>
    </div>
  </div>
{/if}

<svelte:window on:click={() => { userMenuOpen = false; }} />

<style>
  .shell {
    display: flex;
    min-height: 100vh;
  }

  /* ── Sidebar ───────────────────────────────────────────────────────────── */

  .sidebar {
    width: 216px;
    flex-shrink: 0;
    background: #0f1116;
    border-right: 1px solid #1e2128;
    display: flex;
    flex-direction: column;
    position: fixed;
    top: 0;
    left: 0;
    height: 100vh;
    z-index: 200;
    transition: transform 0.22s ease;
    overflow: hidden;
  }

  .sidebar-brand {
    padding: 1.1rem 1.4rem;
    border-bottom: 1px solid #1e2128;
    flex-shrink: 0;
  }

  .brand-link {
    font-size: 1.2rem;
    font-weight: 800;
    color: #4caf50;
    text-decoration: none;
    letter-spacing: -0.03em;
  }

  .sidebar-nav {
    display: flex;
    flex-direction: column;
    padding: 0.5rem 0;
    overflow-y: auto;
    flex: 1;
  }

  .nav-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.58rem 1.4rem;
    color: #7a8aa6;
    text-decoration: none;
    font-size: 0.9rem;
    transition: color 0.1s, background 0.1s;
  }

  .nav-item:hover { color: #c8d0e0; background: #161a22; }

  .nav-item.active {
    color: #4caf50;
    background: #162316;
    font-weight: 500;
  }

  .due-badge {
    background: #2e7d32;
    color: #fff;
    font-size: 0.65rem;
    font-weight: 700;
    padding: 0.12rem 0.4rem;
    border-radius: 10px;
    line-height: 1;
  }

  /* ── Main area ─────────────────────────────────────────────────────────── */

  .shell-main {
    flex: 1;
    margin-left: 216px;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  /* ── Topbar ────────────────────────────────────────────────────────────── */

  .topbar {
    height: 50px;
    background: #13151a;
    border-bottom: 1px solid #1e2128;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 1.25rem;
    position: sticky;
    top: 0;
    z-index: 100;
    flex-shrink: 0;
  }

  .menu-btn {
    background: none;
    border: none;
    color: #7a8aa6;
    font-size: 1.15rem;
    cursor: pointer;
    padding: 0.3rem 0.45rem;
    border-radius: 5px;
    display: none;
    transition: color 0.1s, background 0.1s;
  }
  .menu-btn:hover { color: #e8eaf0; background: #1e2128; }

  .topbar-right {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    margin-left: auto;
  }

  .lang-select {
    background: #1e2128;
    border: 1px solid #2a2d36;
    color: #9ba8c0;
    padding: 0.3rem 0.6rem;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
    outline: none;
    transition: border-color 0.1s;
  }
  .lang-select:focus { border-color: #4caf50; }

  /* ── User menu ─────────────────────────────────────────────────────────── */

  .user-menu-wrap {
    position: relative;
    cursor: pointer;
  }

  .user-btn {
    width: 1.9rem;
    height: 1.9rem;
    border-radius: 50%;
    background: #2e7d32;
    color: #fff;
    border: none;
    font-size: 0.82rem;
    font-weight: 700;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.1s;
  }
  .user-btn:hover { background: #43a047; }

  .user-dropdown {
    position: absolute;
    top: calc(100% + 7px);
    right: 0;
    background: #1e2128;
    border: 1px solid #2a2d36;
    border-radius: 9px;
    min-width: 180px;
    box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
    padding: 0.4rem 0;
    z-index: 300;
  }

  .dropdown-info { padding: 0.5rem 1rem 0.4rem; }
  .dropdown-name { font-size: 0.87rem; font-weight: 600; color: #e8eaf0; }
  .dropdown-email { font-size: 0.74rem; color: #8a96b3; margin-top: 0.1rem; }

  .dropdown-hr { border: none; border-top: 1px solid #2a2d36; margin: 0.3rem 0; }

  .dropdown-item {
    display: block;
    width: 100%;
    padding: 0.45rem 1rem;
    font-size: 0.87rem;
    color: #9ba8c0;
    text-decoration: none;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: color 0.1s, background 0.1s;
  }
  .dropdown-item:hover { color: #e8eaf0; background: #252831; }

  .logout-item { color: #e57373; }
  .logout-item:hover { color: #ef5350; background: #2d1515; }

  /* ── Content ───────────────────────────────────────────────────────────── */

  .shell-content { flex: 1; min-height: 0; }

  /* ── Mobile overlay ────────────────────────────────────────────────────── */

  .sidebar-overlay {
    display: none;
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 150;
  }

  /* ── Responsive ────────────────────────────────────────────────────────── */

  @media (max-width: 768px) {
    .sidebar { transform: translateX(-100%); }
    .shell.sidebar-open .sidebar { transform: translateX(0); }
    .shell.sidebar-open .sidebar-overlay { display: block; }
    .shell-main { margin-left: 0; }
    .menu-btn { display: flex; }
  }
</style>
