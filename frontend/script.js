// script.js: JavaScript for CloudPulse dashboard

async function fetchData() {
    const usageResponse = await fetch('/api/usage');
    const usageData = await usageResponse.json();

    const contributorsResponse = await fetch('/api/contributors');
    const contributors = await contributorsResponse.json();

    updateChart(usageData);
    updateContributors(contributors);
}

function updateChart(data) {
    const ctx = document.getElementById('usageChart').getContext('2d');
    new Chart(ctx, {
        type: 'bar',
        data: {
            labels: ['Free Tier Limit', 'Current Usage'],
            datasets: [{
                label: 'AWS Usage ($)',
                data: [data.freeTierLimit, data.currentUsage],
                backgroundColor: ['#36a2eb', '#ff6384']
            }]
        },
        options: {
            scales: {
                y: { beginAtZero: true }
            }
        }
    });
}

function updateContributors(contributors) {
    const list = document.getElementById('contributorList');
    list.innerHTML = '';
    contributors.forEach(c => {
        const li = document.createElement('li');
        li.textContent = c.login;
        list.appendChild(li);
    });
}

setInterval(fetchData, 60000); // Refresh every minute
fetchData();