fn main() {
    println!("canary");
}

#[cfg(test)]
mod tests {
    #[test]
    fn canary_runs() {
        assert_eq!(2 + 2, 4);
    }
}
