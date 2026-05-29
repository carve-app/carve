<script lang="ts">
</script>

<main>
	<h1>Carve</h1>
	<p>Science-grounded language learning.</p>
	<nav>
		<a href="/cards">Review queue</a>
		<a href="/login">Log in</a>
	</nav>
</main>

<style>
	main {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		font-family: system-ui, sans-serif;
		background: #1e2128;
		color: #e8eaf0;
		gap: 12px;
	}
	h1 {
		font-size: 2rem;
		color: #4caf50;
		margin: 0;
	}
	p {
		color: #6b7a99;
		margin: 0;
	}
	nav {
		display: flex;
		gap: 16px;
		margin-top: 8px;
	}
	nav a {
		color: #4caf50;
		text-decoration: none;
		font-size: 0.95rem;
	}
	nav a:hover {
		text-decoration: underline;
	}
</style>
