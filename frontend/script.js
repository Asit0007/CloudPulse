// script.js - Fetches data and updates the CloudPulse dashboard

document.addEventListener('DOMContentLoaded', () => {
    console.log("CloudPulse Dashboard Initializing...");
    fetchAllData();
    // Optionally, set up a timer to refresh data periodically,
    // but be mindful of API call limits and data transfer for free tier.
    // setInterval(fetchAllData, 300000); // Refresh every 5 minutes (300,000 ms)
});

function showLoading(elementId, message = "Loading data...") {
    const container = document.getElementById(elementId);
    if (container) {
        container.innerHTML = `<p class="loading-text">${message}</p>`;
    }
}

function showError(elementId, errorMessage) {
    const container = document.getElementById(elementId);
    if (container) {
        container.innerHTML = `<p class="error-text">${errorMessage}</p>`;
    }
    console.error(errorMessage);
}

function updateLastUpdated() {
    const now = new Date();
    document.getElementById('last-updated').textContent = now.toLocaleString();
}

async function fetchData(url, options = {}) {
    try {
        // For local development, you might be using http://localhost:8080
        // For production, the Go app serves the frontend, so relative paths work.
        const response = await fetch(url, options);
        if (!response.ok) {
            const errorBody = await response.text();
            throw new Error(`HTTP error! Status: ${response.status}, Body: ${errorBody}`);
        }
        return await response.json();
    } catch (error) {
        console.error(`Failed to fetch from ${url}:`, error);
        throw error; // Re-throw to be caught by the calling function
    }
}

async function fetchEC2Usage() {
    const containerId = 'ec2-data-container';
    showLoading(containerId, 'Loading EC2 instance data...');
    try {
        const data = await fetchData('/api/ec2-usage');
        console.log("EC2 Data Received:", data);
        displayEC2Usage(data);
    } catch (error) {
        showError(containerId, `Error loading EC2 data: ${error.message}. Check backend logs and AWS IAM permissions.`);
    }
}

async function fetchGitHubUsers() {
    const containerId = 'github-data-container';
    showLoading(containerId, 'Loading GitHub repository users...');
    try {
        const users = await fetchData('/api/github-users');
        console.log("GitHub Users Received:", users);
        displayGitHubUsers(users);
    } catch (error) {
        showError(containerId, `Error loading GitHub data: ${error.message}. Check backend logs and GitHub token permissions.`);
    }
}

function displayEC2Usage(data) {
    const ec2DataElement = document.getElementById('ec2-data-container');
    const ec2InstanceIdElement = document.getElementById('ec2-instance-id');

    if (data.error) {
        showError('ec2-data-container', `EC2 API Error: ${data.error}`);
        if(ec2InstanceIdElement) ec2InstanceIdElement.textContent = "Error";
        return;
    }
    if (!data || Object.keys(data).length === 0) {
         showError('ec2-data-container', "No EC2 data returned from API.");
         if(ec2InstanceIdElement) ec2InstanceIdElement.textContent = "N/A";
         return;
    }
    
    if(ec2InstanceIdElement) {
        ec2InstanceIdElement.textContent = data.InstanceID || "N/A";
    }

    // Format CPU to 2 decimal places if available
    const cpuUtilization = data.cpu !== "N/A" && data.cpu !== undefined ? parseFloat(data.cpu).toFixed(2) + "%" : "N/A";
    const networkIn = data.netIn !== "N/A" && data.netIn !== undefined ? parseFloat(data.netIn).toLocaleString() + " bytes" : "N/A";
    const networkOut = data.netOut !== "N/A" && data.netOut !== undefined ? parseFloat(data.netOut).toLocaleString() + " bytes" : "N/A";
    const cpuTimestamp = data.cpu_Timestamp ? new Date(data.cpu_Timestamp).toLocaleTimeString() : "N/A";

    ec2DataElement.innerHTML = `
        <ul>
            <li>CPU Utilization (Avg): <strong>${cpuUtilization}</strong> <em>(at ${cpuTimestamp})</em></li>
            <li>Network In (Sum): <strong>${networkIn}</strong></li>
            <li>Network Out (Sum): <strong>${networkOut}</strong></li>
        </ul>
        ${data.message ? `<p class="loading-text">${data.message}</p>` : ""}
    `;
}

function displayGitHubUsers(users) {
    const githubDataElement = document.getElementById('github-data-container');
    if (users.error) {
        showError('github-data-container', `GitHub API Error: ${users.error}`);
        return;
    }
    if (!users || !Array.isArray(users) || users.length === 0) {
         githubDataElement.innerHTML = `<p>No GitHub users found or access issue.</p>`;
         return;
     }

    let userListHTML = '<ul>';
    users.forEach(user => {
        userListHTML += `
            <li>
                <img src="${user.avatar_url || 'https://placehold.co/30x30/eee/ccc?text=?'}" alt="${user.login || 'user'}" width="30" height="30">
                <a href="${user.html_url || '#'}" target="_blank" rel="noopener noreferrer">${user.login || 'Unknown User'}</a>
                <span>(${user.role_name || 'Collaborator'})</span>
            </li>`;
    });
    userListHTML += '</ul>';

    githubDataElement.innerHTML = userListHTML;
}

function fetchAllData() {
    console.log("Fetching all dashboard data...");
    fetchEC2Usage();
    fetchGitHubUsers();
    // Free Tier data is not fetched via API to avoid costs; it's informational.
    updateLastUpdated();
}

// --- Optimization Notes for Free Tier ---
// 1. Data Fetch: Fetches data on load. Avoid aggressive polling.
//    If auto-refresh is added, make intervals long (e.g., 5-15 minutes).
// 2. Asset Sizes: HTML, CSS, JS are kept minimal. No large frameworks.
// 3. API Responses: Backend Go app should send minimal JSON.
// 4. Error Handling: Graceful error display helps avoid repeated failed calls.
// 5. No Costly APIs: Free Tier data is informational via links, not direct API calls.
