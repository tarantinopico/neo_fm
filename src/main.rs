use anyhow::{anyhow, Result};
use chrono::{DateTime, Local};
use crossterm::{
    event::{self, Event, KeyCode, KeyEventKind, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{
    backend::{Backend, CrosstermBackend},
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Cell, Clear, Paragraph, Row, Table},
    Frame, Terminal,
};
use std::{fs, io::{self, Read, Write}, path::{Path, PathBuf}, time::Duration};
use zip::write::SimpleFileOptions;
use aes_gcm::{aead::{Aead, KeyInit}, Aes256Gcm, Nonce};
use pbkdf2::pbkdf2_hmac;
use sha2::Sha256;
use rand::{RngCore, thread_rng};
use tui_textarea::{TextArea, Input, Key};

const PURPLE: Color = Color::Rgb(145, 71, 255);
const BLUE: Color = Color::Rgb(0, 162, 255);
const GREEN: Color = Color::Rgb(50, 205, 50);
const RED: Color = Color::Rgb(255, 69, 0);
const YELLOW: Color = Color::Rgb(255, 215, 0);
const BG_DARK: Color = Color::Rgb(18, 18, 25);

#[derive(PartialEq, Clone)]
enum AppMode {
    Normal,
    Help,
    Properties,
    Input(InputAction),
    Editor,
}

#[derive(PartialEq, Clone)]
enum InputAction {
    CreateFile,
    CreateDir,
    PasswordEncrypt,
    PasswordDecrypt,
}

struct App<'a> {
    current_dir: PathBuf,
    files: Vec<FileInfo>,
    selected_index: usize,
    mode: AppMode,
    input: String,
    clipboard: Option<PathBuf>,
    status_msg: String,
    editor: TextArea<'a>,
    processing: bool,
}

struct FileInfo {
    name: String,
    path: PathBuf,
    size: u64,
    modified: DateTime<Local>,
    is_dir: bool,
    extension: Option<String>,
}

impl FileInfo {
    fn get_icon(&self) -> &'static str {
        if self.is_dir { "📂" }
        else if let Some(ext) = &self.extension {
            match ext.as_str() {
                "toml" | "conf" | "yaml" | "yml" | "json" => "⚙️",
                "zip" | "7z" | "rar" => "📦",
                "enc" => "🔒",
                _ => "📄",
            }
        } else { "📄" }
    }

    fn get_color(&self) -> Color {
        if self.is_dir { BLUE }
        else if let Some(ext) = &self.extension {
            match ext.as_str() {
                "rs" => GREEN,
                "exe" | "msi" | "bat" => RED,
                "zip" | "rar" | "7z" | "tar" | "gz" => YELLOW,
                "enc" => PURPLE,
                "toml" | "conf" | "yaml" | "yml" | "json" => Color::Rgb(200, 200, 200),
                _ => Color::White,
            }
        } else { Color::White }
    }
}

impl<'a> App<'a> {
    fn new() -> Result<Self> {
        let current_dir = std::env::current_dir()?;
        let mut app = Self {
            current_dir,
            files: Vec::new(),
            selected_index: 0,
            mode: AppMode::Normal,
            input: String::new(),
            clipboard: None,
            status_msg: String::from("Welcome to Neo FM"),
            editor: TextArea::default(),
            processing: false,
        };
        app.refresh_files()?;
        Ok(app)
    }

    fn refresh_files(&mut self) -> Result<()> {
        let mut files = Vec::new();
        if let Some(parent) = self.current_dir.parent() {
            files.push(FileInfo { name: "..".to_string(), path: parent.to_path_buf(), size: 0, modified: Local::now(), is_dir: true, extension: None });
        }
        if let Ok(entries) = fs::read_dir(&self.current_dir) {
            for entry in entries.flatten() {
                let path = entry.path();
                let metadata = entry.metadata().ok();
                let name = entry.file_name().to_string_lossy().to_string();
                let is_dir = metadata.as_ref().map(|m| m.is_dir()).unwrap_or(false);
                let size = metadata.as_ref().map(|m| m.len()).unwrap_or(0);
                let modified = metadata.and_then(|m| m.modified().ok()).map(DateTime::from).unwrap_or_else(Local::now);
                let extension = path.extension().map(|e| e.to_string_lossy().to_lowercase());
                files.push(FileInfo { name, path, size, modified, is_dir, extension });
            }
        }
        files.sort_by(|a, b| {
            if a.name == ".." { std::cmp::Ordering::Less }
            else if b.name == ".." { std::cmp::Ordering::Greater }
            else if a.is_dir != b.is_dir { b.is_dir.cmp(&a.is_dir) }
            else { a.name.to_lowercase().cmp(&b.name.to_lowercase()) }
        });
        self.files = files;
        self.selected_index = self.selected_index.min(self.files.len().saturating_sub(1));
        Ok(())
    }

    fn load_editor(&mut self) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            if !selected.is_dir {
                if let Ok(content) = fs::read_to_string(&selected.path) {
                    self.editor = TextArea::from(content.lines());
                    self.mode = AppMode::Editor;
                    return Ok(());
                } else {
                    self.status_msg = "Could not read file as text".into();
                }
            }
        }
        Ok(())
    }

    fn save_file(&mut self) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            let content = self.editor.lines().join("\n");
            fs::write(&selected.path, content)?;
            self.status_msg = format!("Saved: {}", selected.name);
        }
        Ok(())
    }

    fn delete_selected(&mut self) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            if selected.name == ".." { return Ok(()); }
            if selected.is_dir { fs::remove_dir_all(&selected.path)?; }
            else { fs::remove_file(&selected.path)?; }
            self.status_msg = format!("Deleted: {}", selected.name);
            self.refresh_files()?;
        }
        Ok(())
    }

    fn copy_to_clipboard(&mut self) {
        if let Some(selected) = self.files.get(self.selected_index) {
            if selected.name != ".." {
                self.clipboard = Some(selected.path.clone());
                self.status_msg = format!("Copied: {}", selected.name);
            }
        }
    }

    fn paste_from_clipboard(&mut self) -> Result<()> {
        if let Some(src) = self.clipboard.clone() {
            let dest = self.current_dir.join(src.file_name().unwrap());
            if src.is_dir() { copy_dir_recursive(&src, &dest)?; }
            else { fs::copy(&src, &dest)?; }
            self.status_msg = "Pasted successfully".into();
            self.refresh_files()?;
        }
        Ok(())
    }

    fn zip_selected(&mut self) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            if selected.name == ".." { return Ok(()); }
            self.processing = true;
            let zip_path = self.current_dir.join(format!("{}.zip", selected.name));
            let file = fs::File::create(&zip_path)?;
            let mut zip = zip::ZipWriter::new(file);
            let options = SimpleFileOptions::default().compression_method(zip::CompressionMethod::Deflated);
            if selected.is_dir { self.add_dir_to_zip(&mut zip, &selected.path, &selected.name, options)?; }
            else { zip.start_file(&selected.name, options)?; let mut f = fs::File::open(&selected.path)?; io::copy(&mut f, &mut zip)?; }
            zip.finish()?;
            self.status_msg = format!("Zipped: {}", selected.name);
            self.processing = false;
            self.refresh_files()?;
        }
        Ok(())
    }

    fn add_dir_to_zip(&self, zip: &mut zip::ZipWriter<fs::File>, path: &Path, base_name: &str, options: SimpleFileOptions) -> Result<()> {
        for entry in fs::read_dir(path)? {
            let entry = entry?; let path = entry.path(); let name = format!("{}/{}", base_name, entry.file_name().to_string_lossy());
            if path.is_dir() { zip.add_directory(&name, options)?; self.add_dir_to_zip(zip, &path, &name, options)?; }
            else { zip.start_file(&name, options)?; let mut f = fs::File::open(&path)?; io::copy(&mut f, zip)?; }
        }
        Ok(())
    }

    fn encrypt_selected(&mut self, password: &str) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            if selected.is_dir { return Err(anyhow!("Cannot encrypt dir")); }
            self.processing = true;
            let mut data = Vec::new(); fs::File::open(&selected.path)?.read_to_end(&mut data)?;
            let mut salt = [0u8; 16]; thread_rng().fill_bytes(&mut salt);
            let mut key = [0u8; 32]; pbkdf2_hmac::<Sha256>(password.as_bytes(), &salt, 100_000, &mut key);
            let cipher = Aes256Gcm::new_from_slice(&key).map_err(|_| anyhow!("Bad key"))?;
            let mut nonce_bytes = [0u8; 12]; thread_rng().fill_bytes(&mut nonce_bytes); let nonce = Nonce::from_slice(&nonce_bytes);
            let ciphertext = cipher.encrypt(nonce, data.as_ref()).map_err(|e| anyhow!("Enc error: {:?}", e))?;
            let mut out = fs::File::create(self.current_dir.join(format!("{}.enc", selected.name)))?; out.write_all(&salt)?; out.write_all(&nonce_bytes)?; out.write_all(&ciphertext)?;
            self.status_msg = format!("Encrypted: {}", selected.name);
            self.processing = false;
            self.refresh_files()?;
        }
        Ok(())
    }

    fn decrypt_selected(&mut self, password: &str) -> Result<()> {
        if let Some(selected) = self.files.get(self.selected_index) {
            if !selected.name.ends_with(".enc") { return Err(anyhow!("Not .enc")); }
            self.processing = true;
            let mut file = fs::File::open(&selected.path)?; let mut salt = [0u8; 16]; let mut nonce_bytes = [0u8; 12];
            file.read_exact(&mut salt)?; file.read_exact(&mut nonce_bytes)?; let mut ciphertext = Vec::new(); file.read_to_end(&mut ciphertext)?;
            let mut key = [0u8; 32]; pbkdf2_hmac::<Sha256>(password.as_bytes(), &salt, 100_000, &mut key);
            let cipher = Aes256Gcm::new_from_slice(&key).map_err(|_| anyhow!("Bad key"))?; let nonce = Nonce::from_slice(&nonce_bytes);
            let plaintext = cipher.decrypt(nonce, ciphertext.as_ref()).map_err(|_| anyhow!("Decryption failed"))?;
            let out_name = selected.name.trim_end_matches(".enc"); fs::File::create(self.current_dir.join(out_name))?.write_all(&plaintext)?;
            self.status_msg = format!("Decrypted: {}", out_name);
            self.processing = false;
            self.refresh_files()?;
        }
        Ok(())
    }
}

fn copy_dir_recursive(src: &Path, dest: &Path) -> Result<()> {
    fs::create_dir_all(dest)?;
    for entry in fs::read_dir(src)? {
        let entry = entry?; let path = entry.path();
        if path.is_dir() { copy_dir_recursive(&path, &dest.join(entry.file_name()))?; }
        else { fs::copy(&path, &dest.join(entry.file_name()))?; }
    }
    Ok(())
}

fn main() -> Result<()> {
    enable_raw_mode()?;
    let mut stdout = io::stdout(); execute!(stdout, EnterAlternateScreen)?;
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout))?;
    let mut app = App::new()?;
    let res = run_app(&mut terminal, &mut app);
    disable_raw_mode()?; execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    if let Err(err) = res { println!("Error: {:?}", err); }
    Ok(())
}

fn run_app<B: Backend>(terminal: &mut Terminal<B>, app: &mut App) -> Result<()> {
    loop {
        terminal.draw(|f| ui(f, app))?;
        if event::poll(Duration::from_millis(100))? {
            let ev = event::read()?;
            if let Event::Key(key) = &ev {
                // Windows key fix: only handle Press
                if key.kind != KeyEventKind::Press { continue; }

                if app.mode == AppMode::Editor {
                    if key.code == KeyCode::Char('s') && key.modifiers.contains(KeyModifiers::CONTROL) { app.save_file()?; }
                    else if key.code == KeyCode::Esc || key.code == KeyCode::F(7) { app.mode = AppMode::Normal; }
                    else {
                        let input = map_key(key);
                        app.editor.input(input);
                    }
                    continue;
                }

                if let AppMode::Input(_) = app.mode {
                    match key.code {
                        KeyCode::Enter => {
                            let (action, input) = if let AppMode::Input(a) = &app.mode { (a.clone(), app.input.clone()) } else { unreachable!() };
                            app.mode = AppMode::Normal; app.input.clear();
                            terminal.draw(|f| { app.processing = true; ui(f, app); app.processing = false; })?;
                            match action {
                                InputAction::CreateFile => { fs::File::create(app.current_dir.join(&input))?; app.status_msg = format!("Created: {}", input); app.refresh_files()?; }
                                InputAction::CreateDir => { fs::create_dir(app.current_dir.join(&input))?; app.status_msg = format!("Created: {}", input); app.refresh_files()?; }
                                InputAction::PasswordEncrypt => app.encrypt_selected(&input)?,
                                InputAction::PasswordDecrypt => app.decrypt_selected(&input)?,
                            }
                        }
                        KeyCode::Esc => { app.mode = AppMode::Normal; app.input.clear(); }
                        KeyCode::Char(c) => app.input.push(c),
                        KeyCode::Backspace => { app.input.pop(); }
                        _ => {}
                    }
                    continue;
                }

                match (key.code, key.modifiers) {
                    (KeyCode::Char('q'), _) => return Ok(()),
                    (KeyCode::Esc, _) => app.mode = AppMode::Normal,
                    (KeyCode::F(1), _) => app.mode = AppMode::Help,
                    (KeyCode::F(7), _) => app.load_editor()?,
                    (KeyCode::Up, _) => if app.selected_index > 0 { app.selected_index -= 1; },
                    (KeyCode::Down, _) => if !app.files.is_empty() && app.selected_index < app.files.len() - 1 { app.selected_index += 1; },
                    (KeyCode::Enter, _) => {
                        if let Some(selected) = app.files.get(app.selected_index) {
                            if selected.is_dir { app.current_dir = selected.path.clone(); app.selected_index = 0; app.refresh_files()?; }
                            else { app.load_editor()?; }
                        }
                    }
                    (KeyCode::Char('c'), KeyModifiers::CONTROL) => app.copy_to_clipboard(),
                    (KeyCode::Char('v'), KeyModifiers::CONTROL) => app.paste_from_clipboard()?,
                    (KeyCode::Char('p'), KeyModifiers::CONTROL) => app.mode = AppMode::Properties,
                    (KeyCode::Char('z'), KeyModifiers::CONTROL) => {
                        terminal.draw(|f| { app.processing = true; ui(f, app); app.processing = false; })?;
                        app.zip_selected()?;
                    }
                    (KeyCode::Char('e'), KeyModifiers::CONTROL) => app.mode = AppMode::Input(InputAction::PasswordEncrypt),
                    (KeyCode::Char('d'), KeyModifiers::CONTROL) => app.mode = AppMode::Input(InputAction::PasswordDecrypt),
                    (KeyCode::F(6), _) => app.mode = AppMode::Input(InputAction::CreateFile),
                    (KeyCode::Char('n'), m) if m.contains(KeyModifiers::CONTROL) => app.mode = AppMode::Input(InputAction::CreateDir),
                    (KeyCode::F(8), _) | (KeyCode::Delete, _) => app.delete_selected()?,
                    _ => {}
                }
            }
        }
    }
}

fn ui(f: &mut Frame, app: &App) {
    let size = f.area();
    let chunks = Layout::default().direction(Direction::Vertical).constraints([Constraint::Length(3), Constraint::Min(0), Constraint::Length(1)]).split(size);

    // Path Bar
    f.render_widget(Paragraph::new(format!(" 📍 {} ", app.current_dir.to_string_lossy())).style(Style::default().fg(BLUE).bg(BG_DARK)).block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(PURPLE)).title(" Path ")), chunks[0]);

    if app.mode == AppMode::Editor {
        let mut textarea = app.editor.clone();
        textarea.set_block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(YELLOW)).title(" Editor: ESC/F7 to exit | Ctrl+S to save "));
        f.render_widget(&textarea, chunks[1]);
    } else {
        // Table (Full Width)
        let rows = app.files.iter().enumerate().map(|(i, file)| {
            let style = if i == app.selected_index { Style::default().bg(PURPLE).fg(Color::White) } else { Style::default().fg(file.get_color()) };
            let size_str = if file.is_dir { if file.name == ".." { "".into() } else { "<DIR>".into() } } else { format!("{:.1} KB", file.size as f64 / 1024.0) };
            Row::new(vec![Cell::from(format!("{} {}", file.get_icon(), file.name)), Cell::from(size_str), Cell::from(file.modified.format("%y-%m-%d %H:%M").to_string())]).style(style)
        });
        f.render_widget(Table::new(rows, [Constraint::Percentage(50), Constraint::Percentage(20), Constraint::Percentage(30)]).header(Row::new(vec!["Name", "Size", "Modified"]).style(Style::default().fg(PURPLE).add_modifier(Modifier::BOLD))).block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(PURPLE)).title(" Files ")), chunks[1]);
    }

    // Status Bar
    let status = if app.processing { " ⏳ PROCESSING... " } else { &app.status_msg };
    let clip = if let Some(c) = &app.clipboard { format!(" [CLIP: {}]", c.file_name().unwrap().to_string_lossy()) } else { "".into() };
    f.render_widget(Paragraph::new(format!(" {} {} | F1: Help | F7: Editor | F6: New File | Ctrl+N: New Dir ", status, clip)).style(Style::default().bg(BG_DARK)), chunks[2]);

    // Popups
    match &app.mode {
        AppMode::Help => render_help_popup(f),
        AppMode::Properties => render_properties_popup(f, app),
        AppMode::Input(action) => render_input_popup(f, app, action),
        _ => {}
    }
}

fn map_key(key: &event::KeyEvent) -> Input {
    Input {
        key: match key.code {
            KeyCode::Char(c) => Key::Char(c),
            KeyCode::Backspace => Key::Backspace,
            KeyCode::Enter => Key::Enter,
            KeyCode::Left => Key::Left,
            KeyCode::Right => Key::Right,
            KeyCode::Up => Key::Up,
            KeyCode::Down => Key::Down,
            KeyCode::Tab => Key::Tab,
            KeyCode::Delete => Key::Delete,
            KeyCode::Home => Key::Home,
            KeyCode::End => Key::End,
            KeyCode::PageUp => Key::PageUp,
            KeyCode::PageDown => Key::PageDown,
            KeyCode::Esc => Key::Esc,
            _ => Key::Null,
        },
        ctrl: key.modifiers.contains(KeyModifiers::CONTROL),
        alt: key.modifiers.contains(KeyModifiers::ALT),
        shift: key.modifiers.contains(KeyModifiers::SHIFT),
    }
}

fn render_help_popup(f: &mut Frame) {
    let area = centered_rect(60, 50, f.area()); f.render_widget(Clear, area);
    let help = vec![
        Line::from(vec![Span::styled("Keys", Style::default().fg(PURPLE).add_modifier(Modifier::BOLD))]),
        Line::from(" F7: Fullscreen Editor"),
        Line::from(" F6 / Ctrl+N: New File / Folder"),
        Line::from(" F8 / Delete: Delete selected"),
        Line::from(" Ctrl+C / V: Copy / Paste"),
        Line::from(" Ctrl+P: Properties | Ctrl+Z: Zip"),
        Line::from(" Ctrl+E / D: Encrypt / Decrypt"),
        Line::from(" q / Esc: Quit / Close popup"),
    ];
    f.render_widget(Paragraph::new(help).block(Block::default().title(" Help ").borders(Borders::ALL).border_style(Style::default().fg(PURPLE))), area);
}

fn render_properties_popup(f: &mut Frame, app: &App) {
    if let Some(s) = app.files.get(app.selected_index) {
        let area = centered_rect(70, 40, f.area()); f.render_widget(Clear, area);
        let props = vec![
            Line::from(vec![Span::styled("Properties", Style::default().fg(PURPLE).add_modifier(Modifier::BOLD))]),
            Line::from(format!(" Path: {:?}", s.path)),
            Line::from(format!(" Size: {} bytes", s.size)),
            Line::from(format!(" Mod:  {}", s.modified)),
        ];
        f.render_widget(Paragraph::new(props).block(Block::default().title(" Properties ").borders(Borders::ALL).border_style(Style::default().fg(PURPLE))), area);
    }
}

fn render_input_popup(f: &mut Frame, app: &App, action: &InputAction) {
    let area = centered_rect(50, 20, f.area()); f.render_widget(Clear, area);
    let title = match action { InputAction::CreateFile => " New File ", InputAction::CreateDir => " New Folder ", _ => " Password " };
    let display = if matches!(action, InputAction::PasswordEncrypt | InputAction::PasswordDecrypt) { "*".repeat(app.input.len()) } else { app.input.clone() };
    f.render_widget(Paragraph::new(display).block(Block::default().title(title).borders(Borders::ALL).border_style(Style::default().fg(YELLOW))), area);
}

fn centered_rect(percent_x: u16, percent_y: u16, r: Rect) -> Rect {
    let popup_layout = Layout::default().direction(Direction::Vertical).constraints([Constraint::Percentage((100 - percent_y) / 2), Constraint::Percentage(percent_y), Constraint::Percentage((100 - percent_y) / 2)]).split(r);
    Layout::default().direction(Direction::Horizontal).constraints([Constraint::Percentage((100 - percent_x) / 2), Constraint::Percentage(percent_x), Constraint::Percentage((100 - percent_x) / 2)]).split(popup_layout[1])[1]
}
