document.querySelector('#search-input').addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      performSearch(e.target.value);
    }
  });
  
  document.querySelector('#search-icon').addEventListener('click', function () {
    const query = document.querySelector('#search-input').value;
    performSearch(query);
  });
  
  function performSearch(query) {
    const resultDiv = document.querySelector('#results');
    resultDiv.innerHTML = '<div class="loading">Searching...</div>';
    // Add before fetch call
    query = query.trim().replace(/[<>]/g, '')
    const searchParams = new URLSearchParams()
    searchParams.append('q', query)
    fetch('/api/search?' + searchParams.toString())
      .then(function (response) {
        if (response.ok) {
          return response.json();
        } else {
          throw new Error('Failed to fetch data from the API');
        }
      })
      .then(function (data) {
        displaySearchResults(data);
      })
      .catch(function (error) {
        console.error('Error:', error);
        alert('An error occurred while searching the blockchain. Please try again later.');
      });
  }  
  function displaySearchResults(data) {
    const resultDiv = document.createElement('div');
    resultDiv.innerHTML = '<pre>' + JSON.stringify(data, null, 2) + '</pre>';
    document.body.appendChild(resultDiv);
  }

const debounceSearch = _.debounce(performSearch, 300);

const SELECTORS = {
  searchInput: '#search-input',
  searchIcon: '#search-icon'
};
const API_ENDPOINTS = {
  search: '/api/search'
};
